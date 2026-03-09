# Email

AYB sends transactional emails for password reset, email verification, and magic link login. Three backend options let you start with zero configuration and switch to production-ready email delivery when needed.

## Template customization

AYB supports configurable subject/HTML email templates for built-in auth flows and arbitrary app keys.

- Full guide: [Email Templates](./email-templates.md)
- Admin/API/CLI management: create overrides, preview with variables, enable/disable, reset, and send test emails
- Auth safety behavior: if a custom auth template fails to render, AYB falls back to the built-in default so auth flows continue

Operational workflow:

1. Customize or create a template (`/api/admin/email/templates/{key}` or `ayb email-templates set`)
2. Preview with real variables before saving/sending
3. Send a test email (`/api/admin/email/send` or `ayb email-templates send`) to validate final output

## Backends

### Log (default)

Prints emails to the console. Perfect for development — no setup needed.

```toml
[email]
backend = "log"
```

Password reset and verification links appear in your terminal output.

### SMTP

Send real emails via any SMTP provider.

```toml
[email]
backend = "smtp"
from = "noreply@yourapp.com"
from_name = "YourApp"

[email.smtp]
host = "smtp.resend.com"
port = 465
username = "resend"
password = "re_your_api_key"
auth_method = "PLAIN"
tls = true
```

#### Provider presets

**Resend:**
```toml
[email.smtp]
host = "smtp.resend.com"
port = 465
username = "resend"
password = "re_YOUR_API_KEY"
tls = true
```

**Brevo (Sendinblue):**
```toml
[email.smtp]
host = "smtp-relay.brevo.com"
port = 587
username = "your@email.com"
password = "your-smtp-key"
```

**AWS SES:**
```toml
[email.smtp]
host = "email-smtp.us-east-1.amazonaws.com"
port = 465
username = "AKIAIOSFODNN7EXAMPLE"
password = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
auth_method = "PLAIN"
tls = true
```

### Webhook

POST email data to your own endpoint. Useful for custom email pipelines, logging, or third-party APIs.

```toml
[email]
backend = "webhook"

[email.webhook]
url = "https://your-app.com/api/send-email"
secret = "hmac-signing-secret"
timeout = 10
```

AYB sends a POST request with:

```json
{
  "to": "user@example.com",
  "subject": "Reset your password",
  "html": "<h1>Password Reset</h1>...",
  "text": "Password Reset\n..."
}
```

When `secret` is set, the request includes an `X-AYB-Signature` header with an HMAC-SHA256 signature of the request body.

## Environment variables

All email settings can be configured via environment variables:

```bash
AYB_EMAIL_BACKEND=smtp
AYB_EMAIL_FROM=noreply@yourapp.com
AYB_EMAIL_FROM_NAME=YourApp
AYB_EMAIL_SMTP_HOST=smtp.resend.com
AYB_EMAIL_SMTP_PORT=465
AYB_EMAIL_SMTP_USERNAME=resend
AYB_EMAIL_SMTP_PASSWORD=re_YOUR_API_KEY
AYB_EMAIL_SMTP_TLS=true
```
