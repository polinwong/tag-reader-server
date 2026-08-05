tag-reader-server
=================

The art verify server for NFC **NTAG 424 DNA**. Included AI scan verify sample and custom verify entry (like tag signature verify).

__Related document__
- [NFC Server architecture](./docs/server-architecture.md)
- [Server important function](./docs/server-important-function.md)

### Linux server service config file

> /etc/systemd/system/nfc-server-ip.service
```ini
# The service for Model source server

[Unit]
Description=NFC tag verify server
After=network.target

[Service]
Type=simple
user=nfc-server
Group=nfc-server
WorkingDirectory=/home/ubuntu/nfc-server
ExecStart=/home/ubuntu/nfc-server/sourceserver-linux --https --port 443 --domain nfc.coinllectibles.art --cert /home/ubuntu/nfc-server/nfc_coinllectibles_art.crt --key /home/ubuntu/nfc-server/nfc.coinllectibles.art.key
TimeoutStopSec=15
NonBlocking=true
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

```bash
# Run this command to set auto start
sudo systemctl daemon-reload
sudo systemctl enable nfc-server-ip.service
```

### Using library

- html5-qrcode: ^2.1.3
- vue: ^3.2.26
- quasar: ^2.4.3
- vue-router: ^4.0.12
- vue-i18n: ^9.1.9
- axios: ^0.24.0

### fail2ban config

> /etc/fail2ban/filter.d/nfc-tag-server.conf
```ini
[Definition]
failregex = .*Login failed: <HOST>
# Don't ban company ip
ignoreregex = 202.130.124.130
```

> /etc/fail2ban/jail.d/nfc-tag-server.conf
```ini
# service name
[nfc-tag-server]
# turn on /off
enabled  = true
# ports to ban (numeric or text)
port     = http,https
# filter from previous step
filter   = nfc-tag-server
# file to parse
logpath  = /home/ubuntu/nfc-server/config/info.log
# ban rule:
# 5 times on 1 minute
maxretry = 5
findtime = 60
# ban on 10 minutes
bantime = 600
```

### fail2ban system config
This part is system ban config, included SSH login. If ip have been banned 2 time per a day then will ban a week.
> /etc/fail2ban/jail.d/defaults-debian.conf
```ini
[sshd]
enabled = true
port    = ssh
# ban rule:
# 3 time in 30 minutes
maxretry = 3
findtime = 3600
# ban on 1 hour
bantime = 3600

[f2b-ad]
enabled  = true
port     = http
filter   = f2b-ad
logpath  = /var/log/fail2ban.log
findtime = 86400
bantime  = 604800
maxretry = 2
```