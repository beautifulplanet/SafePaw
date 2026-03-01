# SafePaw — Alert notifications

The SafePaw Grafana stack includes [alert rules](grafana-alerts.yml) for gateway down, auth spikes, rate limiting, prompt injection, error rate, and revocation activity. To get notified when an alert fires, configure **contact points** and optionally **notification policies**.

## Option 1: Configure in Grafana UI (fastest)

1. Start the monitoring stack: `docker compose -f docker-compose.monitoring.yml up -d` (from the safepaw directory).
2. Open Grafana (default http://localhost:3001, admin/admin).
3. Go to **Alerting** → **Contact points** → **New contact point**.
4. Add one or more of:
   - **Email:** Set SMTP in Grafana server config, then add a contact point with type "Email" and your address.
   - **Slack:** Create an [Incoming Webhook](https://api.slack.com/messaging/webhooks), then add contact point type "Slack", paste the webhook URL, and set channel (e.g. `#alerts`).
   - **PagerDuty:** In PagerDuty create an Integration (Events API v2), copy the Integration Key. In Grafana add contact point type "PagerDuty", paste the key.
5. Go to **Alerting** → **Notification policies**. Ensure the default policy routes to your new contact point(s), or add a specific route for the "SafePaw" folder.

## Option 2: Provision contact points with YAML

For repeatable setup, provision contact points via files. Grafana will load any YAML in its alerting provisioning directory.

1. Copy the example and fill in your values (do not commit secrets):

   ```bash
   cp monitoring/contact-points.example.yml monitoring/contact-points.yml
   # Edit contact-points.yml: add webhook URLs, email addresses, or PagerDuty key
   ```

2. Mount your file into the Grafana container. If using `docker-compose.monitoring.yml`, add a volume:

   ```yaml
   volumes:
     - ./grafana-alerts.yml:/etc/grafana/provisioning/alerting/safepaw-alerts.yml:ro
     - ./contact-points.yml:/etc/grafana/provisioning/alerting/contact-points.yml:ro
   ```

3. Restart Grafana. Contact points will appear under Alerting → Contact points.

See [Grafana: Provision alerting resources](https://grafana.com/docs/grafana/latest/alerting/set-up/provision-alerting-resources/) for full syntax.

## Severity and response

| Label / severity | Suggested action |
|------------------|------------------|
| `severity: critical` | Page on-call; mitigate within 15 min (see [RUNBOOK.md](../RUNBOOK.md) Incident response timeline). |
| `severity: warning` | Notify channel; investigate within 1 hour. |
| `severity: info` | Review in runbook; address next business day. |

Each alert rule includes a `runbook_url` annotation pointing to the relevant RUNBOOK section. Use that in your notification message or runbook automation.

## Example: Slack contact point (provisioned)

In `contact-points.yml` (or a similar file in Grafana’s alerting provisioning dir):

```yaml
apiVersion: 1
contactPoints:
  - orgId: 1
    name: safepaw-slack
    receivers:
      - uid: slack-safepaw
        type: slack
        settings:
          url: "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
          recipient: "#alerts"
          title: "{{ .GroupLabels.alertname }}"
          text: "{{ .CommonAnnotations.description }}"
```

Replace `url` with your Slack Incoming Webhook URL and `recipient` with the channel. Do not commit real URLs to the repo; use env vars or a secrets manager in production.

## Example: PagerDuty (critical only)

For P0/P1 alerts only, add a contact point with type `pagerduty` and your Integration Key, then in **Notification policies** add a route that matches `severity=critical` and sends to that contact point.
