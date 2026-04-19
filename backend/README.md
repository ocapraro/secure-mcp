# SMCP Backend
GoLand backend for SMCP


```bash
set -a
source .env
set +a
go run main.go
```


## Sandbox Setup
SMCP relies on QEMU virtualization for plugin sandboxing. For this implementation we are using an arm64 minimal install of Debian and configuring it via UTM. 

### Downloads
[iso](https://cdimage.debian.org/debian-cd/current/arm64/iso-cd/)

### Debian install
Open UTM, and virtualize a new machine from the iso above. Select the `/specialists/shared-scripts` repository as the shared folder and set its permissions to Read Only. This is where the plugins move their scripts to be run. Also create a PTTY serial for the ouput to be directed to.

after setting up the vm, login as root and mount the shared drive:
```bash
mkdir -p /mnt/scripts
mount -t 9p -o trans=virtio,version=9p2000.L,ro share /mnt/scripts
```

then edit add this to `/etc/fstab`
```fstab
share /mnt/scripts 9p trans=virtio,version=9p2000.L,ro,_netdev 0 0
```

Then create `/usr/local/bin/run.sh`
```bash
#!/bin/sh
set -u

STAMP=/run/llm-runner.done
if [ -e "$STAMP" ]; then
  exit 0
fi
touch "$STAMP"

exec > /dev/ttyAMA0 2>&1

ulimit -t 5
ulimit -v 262144

SCRIPT_DIR="/mnt/scripts"

echo '{"type":"vm_status","msg":"runner started"}'

if [ ! -d "$SCRIPT_DIR" ]; then
  printf '%s\n' '{"type":"vm_error","message":"script directory missing"}'
  poweroff -f
  exit 1
fi

found=0

for script in "$SCRIPT_DIR"/*.py; do
  [ -f "$script" ] || continue

  found=1
  name=$(basename "$script")

  output=$(python3 "$script" 2>&1)
  code=$?

  escaped=$(printf '%s' "$output" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')

  ok=false
  [ "$code" -eq 0 ] && ok=true

  printf '%s\n' "{\"type\":\"result\",\"script\":\"$name\",\"ok\":$ok,\"exit_code\":$code,\"output\":$escaped}"
done

if [ "$found" -eq 0 ]; then
  printf '%s\n' '{"type":"result","script":"","ok":true,"exit_code":0,"output":"no scripts found"}'
fi

echo '{"type":"vm_status","msg":"powering off"}'
poweroff -f
```

Make it executable
```bash
chmod +x /usr/local/bin/run.sh
```

Make `/etc/systemd/system/llm-runner.service`

```llm-runner.service
[Unit]
Description=Run mounted Python scripts once
After=multi-user.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/run.sh
RemainAfterExit=true

Restart=no

StandardOutput=journal+console
StandardError=journal+console

[Install]
WantedBy=multi-user.target
```

And enable it
```bash
systemctl enable llm-runner.service
```

Now the vm will run all the python scripts in the shared-scripts folder when it is run.