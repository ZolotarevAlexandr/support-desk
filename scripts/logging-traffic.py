#!/usr/bin/env python3
"""Generate actual API requests; print only non-secret request evidence."""
import argparse
import datetime
import http.cookiejar
import json
import urllib.error
import urllib.request

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--url', default='http://127.0.0.1:18080')
parser.add_argument('--rounds', type=int, default=20)
args = parser.parse_args()
client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
events = []


def request(method, path, data=None, expected=200, authenticated=True):
    req = urllib.request.Request(args.url.rstrip('/') + path,
        data=json.dumps(data).encode() if data is not None else None,
        headers={'Content-Type': 'application/json'}, method=method)
    opener = client if authenticated else urllib.request.build_opener()
    try:
        response = opener.open(req, timeout=15)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        body = response.read()
        expected_statuses = expected if isinstance(expected, tuple) else (expected,)
        if path == '/debug/error' and response.code != 500:
            raise RuntimeError(
                f'/debug/error returned HTTP {response.code} instead of 500. '
                'Rebuild and load support-desk:logs-v2, deploy the updated image, '
                'and reconnect the application port-forward.'
            )
        assert response.code in expected_statuses, (method, path, response.code, body.decode())
        events.append({'method': method, 'path': path, 'status': response.code,
                       'request_id': response.headers['X-Request-ID']})
        return json.loads(body) if body else None


# Check the deployed version before creating any demo data.
request('GET', '/debug/error', expected=500, authenticated=False)

password = 'support-minikube-demo-password'
credentials = {'email': 'logging-lab@example.test', 'password': password}
request('POST', '/api/v1/auth/register', dict(credentials, name='Logging lab user',
    password_confirmation=password, role='user'), expected=(201, 409))
request('POST', '/api/v1/auth/login', credentials)
categories = request('GET', '/api/v1/categories')
if isinstance(categories, dict):
    categories = categories.get('items', categories.get('data', []))
ticket = request('POST', '/api/v1/tickets', {'title': 'Logging lab demonstration',
    'description': 'Verify structured logs, delivery, and LogsQL queries.',
    'category_id': categories[0]['id'], 'priority': 'medium'}, expected=201)
if 'data' in ticket:
    ticket = ticket['data']
request('POST', '/api/v1/tickets/' + str(ticket['id']) + '/comments',
    {'body': 'Test comment for the logging lab.'}, expected=201)
for _ in range(args.rounds):
    request('GET', '/api/v1/tickets')
    request('GET', '/api/v1/categories')
    request('GET', '/api/v1/tickets', expected=401, authenticated=False)
    request('GET', '/debug/error', expected=500, authenticated=False)
print(json.dumps({'generated_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
                  'requests': events}, indent=2))
