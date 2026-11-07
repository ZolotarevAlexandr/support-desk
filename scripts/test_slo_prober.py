import importlib.util
from pathlib import Path
import http.server
import json
import threading
import unittest
import urllib.request

SPEC = importlib.util.spec_from_file_location('prober', Path(__file__).with_name('slo-prober.py'))
PROBER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(PROBER)


class ProberTests(unittest.TestCase):
    def test_failed_creation_does_not_skip_comment_or_action(self):
        seen = []

        class Handler(http.server.BaseHTTPRequestHandler):
            def do_POST(self):
                seen.append(self.path)
                self.rfile.read(int(self.headers.get('Content-Length', 0)))
                if self.path == '/api/v1/tickets':
                    self.send_response(503)
                    result = {'error': 'unavailable'}
                else:
                    self.send_response(201 if self.path.endswith('/comments') else 200)
                    result = {'id': 1}
                self.end_headers()
                self.wfile.write(json.dumps(result).encode())

            def do_DELETE(self):
                self.send_response(204)
                self.end_headers()

            def log_message(self, *_):
                pass

        server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        metrics = PROBER.Metrics()
        probe = PROBER.Probe(f'http://127.0.0.1:{server.server_port}', 'unused', 'unused', 1, metrics)
        probe.ready = True
        probe.fixture = 1
        probe.category = 1
        probe.operator_id = 2
        probe.clients = {role: urllib.request.build_opener() for role in ('user', 'operator')}
        probe.round()
        self.assertEqual(len(seen), 3)
        self.assertEqual(metrics.attempts, {'ticket': 1, 'comment': 1, 'action': 1})
        self.assertEqual(metrics.last_success, {'ticket': 0, 'comment': 1, 'action': 1})
        output = metrics.render().decode()
        self.assertIn('support_probe_duration_seconds_bucket{operation="ticket",le="+Inf"} 1', output)
        self.assertIn('support_probe_duration_seconds_count{operation="ticket"} 1', output)

    def test_transport_failure_is_an_observation(self):
        metrics = PROBER.Metrics()
        probe = PROBER.Probe('http://127.0.0.1:1', 'unused', 'unused', 0.2, metrics)
        probe.category = 1
        probe.clients = {'user': urllib.request.build_opener()}
        self.assertFalse(probe.run_operation('ticket'))
        self.assertEqual(metrics.attempts['ticket'], 1)
        self.assertEqual(metrics.successes['ticket'], 0)

    def test_bootstrap_failure_clears_previous_success(self):
        metrics = PROBER.Metrics()
        for operation in PROBER.OPERATIONS:
            metrics.observe(operation, True, 0.1)
        probe = PROBER.Probe('http://127.0.0.1:1', 'unused', 'unused', 0.2, metrics)
        probe.round()
        self.assertEqual(set(metrics.last_success.values()), {0})

    def test_histogram_buckets_are_cumulative(self):
        metrics = PROBER.Metrics()
        metrics.observe('action', True, 0.2)
        metrics.observe('action', False, 1.5)
        self.assertEqual(metrics.buckets['action', 0.1], 0)
        self.assertEqual(metrics.buckets['action', 1], 1)
        self.assertEqual(metrics.buckets['action', 2], 2)
        self.assertEqual(metrics.buckets['action', float('inf')], 2)


if __name__ == '__main__':
    unittest.main()
