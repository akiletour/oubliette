# Oubliette

A sandbox SMTP server for staging Applications. It holds every Message an Application sends, delivers none of them, and shows them to Members in a web interface.

It is one binary with the interface embedded and all its state in one SQLite file. It reads no configuration file: every setting is a command-line flag or an environment variable.

## Build

```sh
make build
```

This builds the interface and produces `./oubliette`, a static binary built without CGO. It needs Go 1.27 and Node 24.

To build the container image instead, which needs only Docker:

```sh
docker build -t oubliette .
```

## Run in a container

```sh
docker volume create oubliette-data
docker run -d --name oubliette \
  -p 8080:8080 -p 1025:1025 \
  -v oubliette-data:/data \
  -v /etc/oubliette/tls:/tls:ro \
  -e OUBLIETTE_SMTP_TLS_CERT=/tls/tls.crt \
  -e OUBLIETTE_SMTP_TLS_KEY=/tls/tls.key \
  oubliette
```

The image runs as the non-root user 65532 and keeps its data file at `/data/oubliette.db`. A named volume works as is. A host directory mounted on `/data` must be writable by user 65532, and the certificate files must be readable by that user.

On its first start, Oubliette creates an Inbox named "Default" and logs its SMTP Credentials once:

```sh
docker logs oubliette | grep smtp_username
```

Without Docker, run the binary with the same settings: `./oubliette --data /var/lib/oubliette/oubliette.db --smtp-tls-cert tls.crt --smtp-tls-key tls.key serve`.

## Create the first Member

No one can sign in until the Operator creates a Member. Run the `member` command against the same data file, here inside the running container:

```sh
docker exec -it oubliette oubliette member add you@example.com
```

It asks for the password twice. From a script, pass the password on standard input:

```sh
printf '%s\n' "$PASSWORD" | docker exec -i oubliette oubliette member add you@example.com
```

`member list`, `member remove <email>` and `member set-password <email>` manage the other accounts. Without Docker, run `./oubliette --data <file> member add <email>`.

## Settings

Each flag has an environment variable with the `OUBLIETTE_` prefix. A flag wins over its variable. `./oubliette --help` prints the same list.

| Flag | Variable | Default | Meaning |
| --- | --- | --- | --- |
| `--data` | `OUBLIETTE_DATA` | `oubliette.db` | The data file. The image sets it to `/data/oubliette.db`. |
| `--http-addr` | `OUBLIETTE_HTTP_ADDR` | `:8080` | Address the web interface listens on, in plain HTTP. |
| `--smtp-addr` | `OUBLIETTE_SMTP_ADDR` | `:1025` | Address the SMTP server listens on. |
| `--smtp-tls-cert` | `OUBLIETTE_SMTP_TLS_CERT` | none | PEM certificate the SMTP server offers through STARTTLS. |
| `--smtp-tls-key` | `OUBLIETTE_SMTP_TLS_KEY` | none | PEM private key of that certificate. |
| `--smtp-insecure-auth` | `OUBLIETTE_SMTP_INSECURE_AUTH` | `false` | Accept SMTP authentication without encryption. Use it only on a private network. |
| `--smtp-advertised-host` | `OUBLIETTE_SMTP_ADVERTISED_HOST` | the host of the page | SMTP host shown to Members in the Integration tab. |
| `--smtp-advertised-port` | `OUBLIETTE_SMTP_ADVERTISED_PORT` | the listening port | SMTP port shown to Members in the Integration tab. |
| `--retention-days` | `OUBLIETTE_RETENTION_DAYS` | `30` | Days a Message is kept before it is purged, from 1 to 36500. |
| `--inbox-cap` | `OUBLIETTE_INBOX_CAP` | `1000` | Messages an Inbox holds, the oldest leaving first, from 1 to 1000000. |
| `--max-message-mb` | `OUBLIETTE_MAX_MESSAGE_MB` | `25` | Largest Message accepted, in MB of 1,048,576 bytes, from 1 to 100. |

A boolean variable takes `true` or `false`. A value that cannot be read stops the server at start, with a message naming the setting.

## SMTP encryption

The server will not start unless it has a certificate (`--smtp-tls-cert` and `--smtp-tls-key`) or the override `--smtp-insecure-auth`. With a certificate, Applications must use STARTTLS before they authenticate, unless the override is also set: the override wins, and the server logs a warning at start. When you add a certificate to an instance that ran with the override, remove the override. The files are read again at each TLS handshake, so a certificate renewed by the platform (cert-manager, ACME) is used without a restart.

Behind a reverse proxy, SMTP needs its own TCP port. STARTTLS begins in cleartext, so a proxy cannot route it by SNI. There are two setups:

- The proxy passes the TCP connection through, and Oubliette performs STARTTLS with a mounted certificate. This can be a cert-manager Secret, Caddy's PEM files on a shared volume, or a certificate extracted from Traefik's `acme.json`.
- The proxy terminates implicit TLS on port 465 and forwards cleartext to Oubliette, which then runs with `--smtp-insecure-auth` on the private network behind it.

The web interface is served in plain HTTP. Let the reverse proxy terminate TLS for it.

## Supervision

- `GET /healthz` answers `200` without authentication.
- Logs go to standard output, one `key=value` line per event.

## Stopping and backups

On `SIGTERM` or `SIGINT` (`docker stop`), Oubliette:

1. stops accepting connections;
2. closes the SMTP connections that are not sending a Message;
3. lets a Message whose sending has begun (`MAIL FROM` answered) finish, for up to 8 seconds, then closes that connection;
4. closes the data file with every write in it.

A Message still arriving after 8 seconds is not held, its Application gets no `250` and should send it again, and Oubliette exits with status 1. Give the platform a grace period of at least 10 seconds, which is Docker's default.

While the server runs, recent writes live in a `-wal` file next to the data file, so copying the data file alone gives an incomplete or inconsistent backup. To back up, stop the server, copy the data file, then start it again. After a clean stop, the data file holds everything on its own.

```sh
docker stop oubliette
docker run --rm -v oubliette-data:/data -v "$PWD":/backup alpine cp /data/oubliette.db /backup/
docker start oubliette
```

To restore, put the copy back as the data file while the server is stopped.
