# Frontend dev console

Static HTML/CSS/JS console for exercising the API locally.

## Run

1. Set `AELORA_FRONTEND_SIDECAR_ENABLED=true` in `.env`.
2. Start the stack: `make up-local`
3. Open `http://127.0.0.1:8081/`

The API is also available at `http://127.0.0.1:8080`. The console is meant to run through the sidecar so browser requests stay same-origin.

## Notes

- Defaults to local auth mode (`AELORA_AUTH_ENABLED=false`).
- On first load, set Base URL (defaults to current origin), User ID, and Device ID.
- Supports room CRUD, WebSocket send/SYNC/ACK, push-token routes, and a simple load tester.
