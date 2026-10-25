#!/usr/bin/env python3
"""Exercise real Support Desk business operations; print no credentials or cookies."""
import argparse
import collections
import datetime
import http.cookiejar
import json
import os
import urllib.error
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--url', default='http://127.0.0.1:18080')
    parser.add_argument('--rounds', type=int, default=4)
    parser.add_argument('--operator-invite', default=os.environ.get('OPERATOR_INVITE_CODE', 'support-minikube-operator'))
    args = parser.parse_args()
    if args.rounds < 1:
        parser.error('--rounds must be positive')
    counts = collections.Counter()

    def request(client, method, path, data=None, expected=(200,)):
        req = urllib.request.Request(args.url.rstrip('/') + path, method=method,
            data=json.dumps(data).encode() if data is not None else None,
            headers={'Content-Type': 'application/json'})
        try:
            response = client.open(req, timeout=15)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            raw = response.read()
            if response.code not in expected:
                raise RuntimeError(f'{method} {path}: HTTP {response.code}, expected {expected}')
            counts[str(response.code)] += 1
            return json.loads(raw) if raw else None

    clients = {}
    users = {}
    for role in ('user', 'operator'):
        client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        credentials = {'email': f'metrics-lab-{role}@example.test', 'password': 'support-minikube-demo-password'}
        request(client, 'POST', '/api/v1/auth/register', dict(credentials, name=f'Metrics lab {role}',
            password_confirmation=credentials['password'], role=role,
            **({'invite_code': args.operator_invite} if role == 'operator' else {})), expected=(201, 409))
        users[role] = request(client, 'POST', '/api/v1/auth/login', credentials)
        clients[role] = client
    request(clients['user'], 'POST', '/api/v1/auth/login',
        {'email': 'metrics-lab-user@example.test', 'password': 'incorrect-password'}, expected=(401,))
    categories = request(clients['user'], 'GET', '/api/v1/categories')
    if not categories:
        raise RuntimeError('No categories; run the seed Job first')

    ticket_ids = []
    for index in range(args.rounds):
        ticket = request(clients['user'], 'POST', '/api/v1/tickets', {
            'title': 'Metrics lab demonstration', 'description': 'Verify counters and resolution histograms.',
            'category_id': categories[0]['id'], 'priority': ('low', 'medium', 'high', 'urgent')[index % 4]}, expected=(201,))
        ticket_ids.append(ticket['id'])
        path = f"/api/v1/tickets/{ticket['id']}"
        for role in ('user', 'operator'):
            request(clients[role], 'POST', path + '/comments', {'body': f'Explicit {role} comment.'}, expected=(201,))

        def action(role, name, status=200, **fields):
            return request(clients[role], 'POST', path + '/actions', dict(action=name, **fields), expected=(status,))

        action('user', 'claim', 403)
        action('operator', 'claim')
        action('operator', 'claim', 409)
        action('operator', 'reassign', assignee_id=users['operator']['id'])
        action('operator', 'resolve', message='Initial resolution.')
        action('user', 'request_changes', message='Further work needed.')
        action('operator', 'resolve', message='Feedback addressed.')
        action('user', 'confirm')
        action('user', 'confirm', 409)
        action('operator', 'reopen', message='Additional investigation.')
        action('operator', 'resolve', message='Final resolution.')
        action('user', 'confirm')
        action('operator', 'resolve', 409, message='Rejected duplicate.')

    print(json.dumps({
        'generated_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
        'rounds': args.rounds, 'ticket_ids': ticket_ids, 'http_status_counts': dict(counts),
        'expected_business_increments': {
            'tickets_opened': args.rounds, 'comments_added_per_role': args.rounds,
            'logins_per_role': 1, 'resolution_observations': 3 * args.rounds,
            'actions': {name: per_round * args.rounds for name, per_round in {
                'claim': 1, 'reassign': 1, 'resolve': 3, 'request_changes': 1, 'confirm': 2, 'reopen': 1}.items()}
        }
    }, indent=2))


if __name__ == '__main__':
    main()
