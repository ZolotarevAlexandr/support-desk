#!/usr/bin/env python3
"""Black-box Support Desk business probes, with a stdlib Prometheus endpoint."""
import argparse
import collections
import http.cookiejar
import http.server
import json
import math
import os
import signal
import threading
import time
import urllib.error
import urllib.request

OPERATIONS = ('ticket', 'comment', 'action')
BUCKETS = (0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, math.inf)


class ProbeError(RuntimeError):
    def __init__(self, status):
        super().__init__('Unexpected probe response status')
        self.code = status


class Metrics:
    def __init__(self):
        self.lock = threading.Lock()
        self.attempts = collections.Counter()
        self.successes = collections.Counter()
        self.last_success = collections.Counter()
        self.durations = collections.Counter()
        self.buckets = collections.Counter()
        self.last_round = 0
        self.started = time.time()

    def observe(self, operation, success, duration):
        with self.lock:
            self.attempts[operation] += 1
            self.successes[operation] += int(success)
            self.last_success[operation] = int(success)
            self.durations[operation] += duration
            for bucket in BUCKETS:
                self.buckets[operation, bucket] += int(duration <= bucket)

    def render(self):
        with self.lock:
            lines = []
            for name, kind, help_text in (
                ('support_probe_attempts_total', 'counter', 'Completed synthetic business requests.'),
                ('support_probe_successes_total', 'counter', 'Successful synthetic business requests.'),
                ('support_probe_success', 'gauge', 'Latest result per business operation, zero before first probe.'),
                ('support_probe_duration_seconds', 'histogram', 'End-to-end business request duration, including failures.'),
                ('support_probe_last_round_timestamp_seconds', 'gauge', 'Last completed round, including bootstrap failures.'),
                ('support_probe_started_timestamp_seconds', 'gauge', 'Prober process start time.'),
            ):
                lines += [f'# HELP {name} {help_text}', f'# TYPE {name} {kind}']
                if name.endswith('timestamp_seconds'):
                    value = self.last_round if 'last_round' in name else self.started
                    lines.append(f'{name} {value}')
                    continue
                for operation in OPERATIONS:
                    labels = f'operation="{operation}"'
                    if kind == 'histogram':
                        for bucket in BUCKETS:
                            boundary = '+Inf' if math.isinf(bucket) else str(bucket)
                            lines.append(f'{name}_bucket{{{labels},le="{boundary}"}} {self.buckets[operation, bucket]}')
                        lines += [f'{name}_count{{{labels}}} {self.attempts[operation]}',
                                  f'{name}_sum{{{labels}}} {self.durations[operation]}']
                    else:
                        values = {'support_probe_attempts_total': self.attempts,
                                  'support_probe_successes_total': self.successes,
                                  'support_probe_success': self.last_success}[name]
                        lines.append(f'{name}{{{labels}}} {values[operation]}')
            return ('\n'.join(lines) + '\n').encode()


class Probe:
    def __init__(self, url, password, invite, timeout, metrics):
        self.url = url.rstrip('/')
        self.password = password
        self.invite = invite
        self.timeout = timeout
        self.metrics = metrics
        self.clients = {}
        self.operator_id = None
        self.fixture = None
        self.category = None
        self.ready = False

    def request(self, role, method, path, data=None, expected=(200,)):
        req = urllib.request.Request(self.url + path, method=method,
            data=json.dumps(data).encode() if data is not None else None,
            headers={'Content-Type': 'application/json'})
        try:
            response = self.clients[role].open(req, timeout=self.timeout)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            raw = response.read()
            if response.status not in expected:
                raise ProbeError(response.status)
            return json.loads(raw) if raw else None

    def bootstrap(self):
        for role in ('user', 'operator'):
            self.clients[role] = urllib.request.build_opener(
                urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
            credentials = {'email': f'slo-lab-{role}@example.test', 'password': self.password}
            self.request(role, 'POST', '/api/v1/auth/register', dict(credentials,
                name=f'SLO probe {role}', password_confirmation=self.password, role=role,
                **({'invite_code': self.invite} if role == 'operator' else {})), expected=(201, 409))
            user = self.request(role, 'POST', '/api/v1/auth/login', credentials)
            if role == 'operator':
                self.operator_id = user['id']
        categories = self.request('user', 'GET', '/api/v1/categories')
        if not categories:
            raise RuntimeError('Run category seed Job first')
        self.category = categories[0]['id']
        if self.fixture is None:
            # This fixture belongs to the synthetic user. Reuse it on restarts.
            page = self.request('user', 'GET', '/api/v1/tickets?search=SLO%20probe%20fixture&page_size=100')
            fixture = next((t for t in page['items'] if t['title'] == 'SLO probe fixture' and t['status'] != 'closed'), None)
            if fixture is None:
                fixture = self.request('user', 'POST', '/api/v1/tickets', self.ticket_input('SLO probe fixture'), (201,))
            self.fixture = fixture['id']
        self.ready = True

    def ticket_input(self, title):
        return {'title': title, 'description': 'Synthetic SLO lab transaction.',
                'category_id': self.category, 'priority': 'medium'}

    def run_operation(self, operation):
        start = time.monotonic()
        success = False
        result = None
        try:
            if operation == 'ticket':
                result = self.request('user', 'POST', '/api/v1/tickets', self.ticket_input('SLO probe transient ticket'), (201,))
            elif operation == 'comment':
                result = self.request('user', 'POST', f'/api/v1/tickets/{self.fixture}/comments',
                                      {'body': 'Synthetic SLO probe comment.'}, (201,))
            else:
                self.request('operator', 'POST', f'/api/v1/tickets/{self.fixture}/actions',
                             {'action': 'reassign', 'assignee_id': self.operator_id})
            success = True
        except Exception as error:
            # Never log response bodies, credentials, cookies, or exception URLs.
            print(json.dumps({'operation': operation, 'success': False,
                              'error_type': type(error).__name__, 'status': getattr(error, 'code', 0)}), flush=True)
            if getattr(error, 'code', None) in (401, 404):
                self.ready = False
                if getattr(error, 'code', None) == 404:
                    self.fixture = None
        finally:
            self.metrics.observe(operation, success, time.monotonic() - start)
        # Cleanup is outside the timed request and does not hit selected counters.
        if success and operation in ('ticket', 'comment'):
            path = f'/api/v1/tickets/{result["id"]}' if operation == 'ticket' else f'/api/v1/tickets/{self.fixture}/comments/{result["id"]}'
            try:
                self.request('user', 'DELETE', path, expected=(204,))
            except Exception:
                print(json.dumps({'operation': operation, 'cleanup_failed': True}), flush=True)
        return success

    def round(self):
        if not self.ready:
            try:
                self.bootstrap()
            except Exception as error:
                with self.metrics.lock:
                    for operation in OPERATIONS:
                        self.metrics.last_success[operation] = 0
                print(json.dumps({'bootstrap': False, 'error_type': type(error).__name__}), flush=True)
                return
        results = {operation: self.run_operation(operation) for operation in OPERATIONS}
        print(json.dumps({'timestamp': time.time(), 'results': results}), flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--url', default=os.environ.get('PROBE_URL', 'http://support-app.support-desk.svc.cluster.local:8080'))
    parser.add_argument('--interval', type=float, default=float(os.environ.get('PROBE_INTERVAL', '10')))
    parser.add_argument('--timeout', type=float, default=7)
    parser.add_argument('--port', type=int, default=9115)
    args = parser.parse_args()
    if args.interval <= 0 or args.timeout <= 0:
        parser.error('interval and timeout must be positive')
    password = os.environ['PROBE_PASSWORD']
    invite = os.environ['OPERATOR_INVITE_CODE']
    observations = Metrics()

    class Handler(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            if self.path not in ('/metrics', '/healthz'):
                self.send_error(404)
                return
            body = observations.render() if self.path == '/metrics' else b'ok\n'
            self.send_response(200)
            self.send_header('Content-Type', 'text/plain; version=0.0.4; charset=utf-8')
            self.send_header('Content-Length', str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, *_):
            pass

    server = http.server.ThreadingHTTPServer(('0.0.0.0', args.port), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    stop = threading.Event()
    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, lambda *_: stop.set())
    probe = Probe(args.url, password, invite, args.timeout, observations)
    while not stop.is_set():
        started = time.monotonic()
        probe.round()
        with observations.lock:
            observations.last_round = time.time()
        stop.wait(max(0, args.interval - (time.monotonic() - started)))
    server.shutdown()


if __name__ == '__main__':
    main()
