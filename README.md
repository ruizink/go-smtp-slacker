# Go SMTP Slacker

A simple, lightweight SMTP server that receives emails and forwards them as direct messages to Slack users. It's designed to be a simple bridge for applications that can only send notifications via email, allowing them to post messages to Slack instead.

## Features

* Acts as a standard SMTP server.
* Parses incoming emails for sender, recipients, subject, and body.
* Forwards formatted messages as direct messages to Slack users, looking them up by the recipient email address.
* Supports optional SMTP `PLAIN` authentication.
* Filters sender and recipient addresses using flexible allow/deny lists.

## Configuration

The server is configured using a YAML file (e.g., `config.yaml`). The file has two main sections: `smtp`, and `slack`.

### Example `config.yaml`

```yaml
smtp:
  listen-addr: "0.0.0.0:1025"
  auth:
    enabled: true
    user-database: "/etc/go-smtp-slacker/users.db"
  policies:
    from:
      default-action: "deny" # "allow" or "deny"
      allow:
        - "alerts@example.com"
        - "monitoring-?@my-domain.com"
      deny:
        - "*@spam.com"
    to:
      default-action: "allow"
      allow:
        - "*"
      deny:
        - "ignored@example.com"
slack:
  token: "xoxb-your-slack-bot-token"
```

---

### `smtp` Section

This section configures the core SMTP server.

* `addr`: The IP address and port the server should listen on. Use `0.0.0.0` to listen on all network interfaces.
* `prefer-html-body`: Set to `true` to use the HTML body from email, if available, otherwise use plain text.

#### `smtp.auth` Section

This section controls SMTP authentication. When enabled, clients must authenticate before they can send an email.

* `enabled`: Set to `true` to require client authentication. If `false`, the server will operate as an open relay (though still subject to policy restrictions).
* `user-database`: The absolute path to the user database file. This file contains the usernames and their corresponding bcrypt-hashed passwords.

##### User Database File

The user database is a simple text file where each line represents a user in the format `username:bcrypt_hash`. Lines starting with `#` are treated as comments and are ignored.

**Example `users.db`:**

```text
# User for our monitoring system
monitoring:$2a$10$abcdefghijklmnopqrstuv.abcdefghijklmnopqrstuv.abcdefghij
```

You can generate a bcrypt hash for a password using various tools. For example, with `htpasswd` (from Apache utils):

```bash
# Install apache2-utils if you don't have it
# Debian/Ubuntu: sudo apt-get install apache2-utils
# CentOS/RHEL:   sudo dnf install httpd-tools

htpasswd -B -n newuser

# The command will ask you for a password and will output something like:
# newuser:$2y$10$P.gL4v2hergM3sU3B5t25eRzWpE7YzzHSHx4xX3jXyS.i3cZz3n.O
#
# Copy the output into your users.db file.
```

#### `smtp.policies` Section

Policies allow you to control which emails are accepted based on sender (`from`) and recipient (`to`) addresses. This is useful for preventing unauthorized use.

**Rules are processed in this order:**

1. If an address matches a pattern in the `deny` list, it is **rejected**.
2. If an address matches a pattern in the `allow` list, it is **accepted**.
3. If an address matches no patterns, the `default-action` is applied.

**Fields:**

* `default-action`: Determines the behavior for an address that does not match any `allow` or `deny` patterns. Can be `allow` or `deny`.
* `allow`: A list of glob patterns. Addresses matching these patterns are allowed.
* `deny`: A list of glob patterns. Addresses matching these patterns are denied.

**Glob Patterns:**
The matching is case-insensitive.

* `*`: Matches any sequence of characters.
* `?`: Matches any single character.

**Example:**

* `*@example.com` matches any user at `example.com`.
* `user-?@domain.com` matches `user-1@domain.com`, `user-a@domain.com`, etc.

### `slack` Section

This section configures the Slack integration.

* `token`: The Slack Bot User OAuth Token for your Slack app. It usually starts with `xoxb-`. This is a **required** field. It can be set via the `SLACK_TOKEN` environment variable or a file specified with `--slack.token-file`.

## Command-Line Flags

Flags can be used to override settings from the configuration file.

| Flag | Shorthand | Description | Default |
|---|---|---|---|
| `--config-file` | | The path to look for the configuration file (`config.yaml`). | `./config.yaml` |
| `--log-level` | | The log level to use. | `INFO` |
| `--smtp.listen-addr` | | Listen address for the SMTP server. | `localhost:25` |
| `--smtp.auth.enabled` | `-a` | Enable SMTP authentication. | `false` |
| `--smtp.auth.user-database` | | Path to the user database file. | |
| `--smtp.prefer-html-body` | | Use HTML from email, if available, otherwise use plain text | `true` |
| `--slack.token-file` | | The path to a file containing the Slack token. | |
| `--help` | `-h` | Prints this help message. | |
| `--version` | `-V` | Prints the version. | |

## Environment Variables

A few key settings can be provided via environment variables for convenience, especially in containerized environments.

| Variable | Description |
|---|---|
| `LOG_LEVEL` | Overrides the `log-level` configuration. |
| `SLACK_TOKEN` | Overrides the `slack.token` configuration. This is the most common way to provide the token securely. |
