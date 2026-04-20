# Secure MCP (SMCP)

SMCP replaces MCP servers with a self-contained multi-agent orchestration system. Instead of delegating code execution to a remotely-hosted third party, SMCP runs specialist plugins inside a local QEMU virtual machine — giving you the same tool-use capabilities as an MCP server without handing execution authority outside your machine.

## How it works

```
User message
    │
    ▼
Planner (HEAVY_MODEL)
  Breaks the request into concrete sub-tasks
    │
    ▼
Delegator (HEAVY_MODEL)
  Assigns each task to the right Specialist or to the Generalist
    │
    ├──► Generalist (LIGHT_MODEL) — handles tasks that don't require a plugin
    │
    └──► Specialist (LIGHT_MODEL) — picks the right plugin script(s) for the task
              │
              ▼
         Staged Python scripts (with resolved arguments + injected SCRIPT_ID)
              │
              ▼
         QEMU sandbox (arm64 Debian, snapshot mode, network isolated)
              │
              ▼
         Results read from /dev/ttyAMA0 serial, matched back to staged scripts
              │
              ▼
Synthesizer — combines all task results into a final response
```

### Specialists

Specialists are defined as XML bios in `backend/specialists/<name>/bio.xml`. Each bio lists the specialist's personality, plugin scripts, argument definitions, and examples. The orchestrator reads these at startup — adding a new specialist is just adding a new directory.

The included **Weather Man** specialist has two plugins:
- `get-weather <location>` — current conditions (temperature, humidity, wind, UV index)
- `fetch-forecast <location>` — 3-day hourly forecast

## Repository layout

```
backend/
  src/                  Go backend (API server + orchestration)
    agents/             Planner, Delegator, Specialist runner, script staging
    sandbox/            QEMU launcher and VM message parser
    server/             HTTP server and orchestration entry point
    openai/             OpenAI API client
    database/           SQLite session store
  specialists/          Specialist definitions and plugin scripts
  shared-scripts/       Staging directory — scripts are copied here before sandbox boot
  data/                 sandbox.qcow2 VM disk image
frontend/
  src/                  Vite + TypeScript chat UI
```

## Requirements

- Go 1.26+
- Node.js 20+ (frontend only)
- QEMU (`brew install qemu` on macOS)
- LLM API key (OpenAI, or Ollama)

## Backend setup

```bash
cd backend/src
set -a && source .env && set +a
go run main.go
```

The server listens on `:8080`.

## Frontend setup

```bash
cd frontend
npm install
npm run dev
```

## Sandbox setup

The sandbox is a minimal arm64 Debian VM that runs staged plugin scripts and streams results back over a virtual serial port (`/dev/ttyAMA0`).

### 1. Download a Debian arm64 ISO

https://cdimage.debian.org/debian-cd/current/arm64/iso-cd/

### 2. Create the VM

Boot the ISO under QEMU (or UTM on macOS). Configure:
- **Shared folder**: point to `backend/shared-scripts/` — mount tag `share`, read-only
- **Serial output**: PTTY serial mapped to `/dev/ttyAMA0` inside the guest

### 3. Inside the guest

Mount the shared drive and persist it:

```bash
mkdir -p /mnt/scripts
mount -t 9p -o trans=virtio,version=9p2000.L,ro share /mnt/scripts

# Add to /etc/fstab:
share /mnt/scripts 9p trans=virtio,version=9p2000.L,ro,_netdev 0 0
```

Create `/usr/local/bin/run.sh`:

```sh
#!/bin/sh
set -u

STAMP=/run/llm-runner.done
[ -e "$STAMP" ] && exit 0
touch "$STAMP"

exec > /dev/ttyAMA0 2>&1

ulimit -t 5
ulimit -v 262144

if ip link show eth0 >/dev/null 2>&1; then
  ip link set eth0 up || true
  dhcpcd eth0 || true
  sleep 1
fi

SCRIPT_DIR="/mnt/scripts"
echo '{"type":"vm_status","msg":"runner started"}'

if [ ! -d "$SCRIPT_DIR" ]; then
  printf '%s\n' '{"type":"vm_error","message":"script directory missing"}'
  poweroff -f; exit 1
fi

found=0
for script in "$SCRIPT_DIR"/*.py; do
  [ -f "$script" ] || continue
  found=1
  name=$(basename "$script")
  printf '%s\n' "{\"type\":\"script_start\",\"script\":\"$name\"}"
  python3 "$script"
  code=$?
  ok=false; [ "$code" -eq 0 ] && ok=true
  printf '%s\n' "{\"type\":\"result\",\"script\":\"$name\",\"ok\":$ok,\"exit_code\":$code}"
done

[ "$found" -eq 0 ] && printf '%s\n' '{"type":"result","script":"","ok":true,"exit_code":0,"output":"no scripts found"}'

echo '{"type":"vm_status","msg":"powering off"}'
poweroff -f
```

```bash
chmod +x /usr/local/bin/run.sh
```

Create `/etc/systemd/system/llm-runner.service`:

```ini
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

```bash
systemctl enable llm-runner.service
```

### 4. Export the disk image

Shut down the VM and export the disk as `backend/data/sandbox.qcow2`.

## Writing a specialist

1. Create `backend/specialists/<your-name>/`
2. Add `bio.xml` — see `weather-man/bio.xml` as a reference
3. Add plugin scripts under `scripts/`. Use `__TOKEN_<argname>:string__` placeholders for arguments and emit results as:

```python
SCRIPT_ID = "__SCRIPT_ID__"   # injected at staging time

emit({
    "type": "result",
    "script": SCRIPT_ID,
    "ok": True,
    "output": json.dumps(your_data),
})
```

The orchestrator injects the correct staged filename into `SCRIPT_ID` before running, so results are automatically matched back to the right task.

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `OPENAI_API_KEY` | *(required)* | OpenAI API key |
| `SANDBOX_TIMEOUT_SECONDS` | `240` | Max seconds to wait for the VM |
| `SANDBOX_SHARED_SCRIPTS_DIR` | `../shared-scripts` | Path where staged scripts are written |
 
