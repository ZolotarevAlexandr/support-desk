#!/usr/bin/env python3
"""Render the Grafana SLO dashboard and its provisioned Kubernetes ConfigMap."""
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
PAIRS = {
    'ticket': ('support_tickets_opened_total', 'support_ticket_open_attempts_total'),
    'comment': ('support_comments_added_total', 'support_comment_add_attempts_total'),
    'action': ('support_ticket_actions_total', 'support_ticket_action_attempts_total'),
}


def queries():
    q = {}
    for operation, (success, attempts) in PAIRS.items():
        numerator = f'sum(increase({success}{{job="support-desk"}}[5m]))'
        denominator = f'sum(increase({attempts}{{job="support-desk"}}[5m]))'
        q[operation] = f'(({numerator} / {denominator}) and ({denominator} > 0)) or vector(0)'
    q['latency'] = 'histogram_quantile(0.95, sum by (le) (rate(support_probe_duration_seconds_bucket{job="slo-prober"}[5m])))'
    q['probe_ratio'] = '(sum(increase(support_probe_successes_total{job="slo-prober"}[5m])) / sum(increase(support_probe_attempts_total{job="slo-prober"}[5m])))'
    gates = [f'(({q[operation]}) >= bool 0.99)' for operation in PAIRS]
    gates += [f'(({q["latency"]}) < bool 1)',
              '(max(up{job="slo-prober"}) == bool 1)',
              '(max(up{job="support-desk"}) == bool 1)',
              '((time() - max(support_probe_last_round_timestamp_seconds{job="slo-prober"})) < bool 45)',
              '(min(support_probe_success{job="slo-prober"}) == bool 1)',
              '(count(support_probe_success{job="slo-prober"}) == bool 3)']
    q['good'] = '(' + ' * '.join(gates) + ') or vector(0)'
    q['sla'] = f'100 * avg_over_time(({q["good"]})[30m:15s])'
    q['budget'] = f'18 - (1800 * (1 - ({q["sla"]}) / 100))'
    q['coverage'] = '100 * clamp_max(((count_over_time((max(up{job="slo-prober"}) == 1)[30m:15s])) or vector(0)) / 120, 1)'
    return q


def dashboard():
    q = queries()
    ds = {'type': 'prometheus', 'uid': 'victoriametrics'}
    panels = []

    def panel(title, exprs, unit, x, y, w=12, threshold=None, good='above', description=''):
        defaults = {'unit': unit, 'decimals': 2}
        if threshold is not None:
            if good == 'above':
                steps = [
                    {'color': 'red', 'value': None},
                    {'color': 'green', 'value': threshold},
                ]
            else:
                steps = [
                    {'color': 'green', 'value': None},
                    {'color': 'red', 'value': threshold},
                ]
            defaults['thresholds'] = {'mode': 'absolute', 'steps': steps}
            defaults['custom'] = {'thresholdsStyle': {'mode': 'line'}}
        panels.append({'id': len(panels) + 1, 'title': title, 'type': 'timeseries', 'datasource': ds,
            'description': description, 'gridPos': {'x': x, 'y': y, 'w': w, 'h': 8},
            'targets': [{'refId': chr(65+i), 'expr': expression, 'legendFormat': legend,
                         'interval': '15s', 'datasource': ds} for i, (legend, expression) in enumerate(exprs)],
            'fieldConfig': {'defaults': defaults, 'overrides': []},
            'options': {'legend': {'displayMode': 'table', 'placement': 'bottom', 'calcs': ['lastNotNull']},
                        'tooltip': {'mode': 'multi'}}})
    panel('Business SLI / SLO ≥ 99% · rolling 5m',
          [(op, f'100 * ({q[op]})') for op in PAIRS], 'percent', 0, 0, threshold=99,
          description='Three existing business counters divided by matching completed eligible attempts. Zero with no eligible observations.')
    panel('Synthetic request p95 / SLO < 1s · rolling 5m', [('p95', q['latency'])], 's', 12, 0,
          threshold=1, good='below')
    panel('Combined SLA ≥ 99% · rolling 30m', [('SLA', q['sla'])], 'percent', 0, 8, threshold=99,
          description='Fixed 15s evaluation grid. Missing or stale probes are bad intervals. Prewarm 30m. SLI recovery still needs five minutes to expire old failures.')
    panel('Remaining 30m error budget · allowance 18s', [('remaining', q['budget'])], 's', 12, 8,
          threshold=0, description='The line at 0s marks budget exhaustion. Negative values mean overspend. One bad 15s slot consumes most of the 18s allowance.')
    panel('Latest interval compliance and latest business probes',
          [('all SLOs good', q['good']), ('{{operation}} probe', 'support_probe_success{job="slo-prober"}')], 'short', 0, 16,
          threshold=1, description='The line at 1 marks a fully good interval and a successful result for every business probe.')
    panel('Independent black-box request success · 5m', [('probe success %', f'100 * ({q["probe_ratio"]})'),
          ('scrape coverage %', q['coverage'])], 'percent', 12, 16,
          threshold=99, description='The line at 99% is the request-based target. Black-box results include connection failures, timeouts and 4xx; application metrics cannot observe requests that never reach it.')
    raw = []
    for op, (success, attempts) in PAIRS.items():
        raw.extend([(f'{op} successes', f'sum(increase({success}{{job="support-desk"}}[5m]))'),
                    (f'{op} attempts', f'sum(increase({attempts}{{job="support-desk"}}[5m]))')])
    panel('Raw business operations · rolling 5m increases · zero reference', raw, 'short', 0, 24, w=24,
          threshold=0, description='The zero line is a diagnostic reference; SLO targets are shown in the business SLI panel above.')
    return {'uid': 'support-slo', 'title': 'Support Desk — SLI / SLO / SLA', 'schemaVersion': 41,
            'version': 1, 'timezone': 'utc', 'refresh': '15s', 'time': {'from': 'now-45m', 'to': 'now'},
            'tags': ['slo-lab'], 'panels': panels}


def main():
    data = json.dumps(dashboard(), indent=2, ensure_ascii=False) + '\n'
    (ROOT / 'docs/slo/dashboard.json').write_text(data)
    (ROOT / 'docs/slo/queries.json').write_text(json.dumps(queries(), indent=2) + '\n')
    (ROOT / 'k8s/slo/grafana-dashboard.yaml').write_text(
        'apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: grafana-slo-dashboard\n  namespace: logging\ndata:\n  support-slo.json: |\n'
        + ''.join('    ' + line + '\n' for line in data.splitlines()))


if __name__ == '__main__':
    main()
