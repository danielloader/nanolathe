# Nanolathe relay

Small, standalone relay for Nanolathe's first hosted two-player play test.
The clients simulate the battle; this service forwards commands and tick grants.
No game assets, GPU, database, or game engine are needed on the server.

This repository is private. Its deployed endpoint is reachable publicly; room
codes are invitations, not user accounts. This is the two-human Modern skirmish
prototype, with no reconnect, AI, spectators or distributed room directory.

## Deploy on DigitalOcean App Platform

The live service is `https://relay.nanolathe.gg` (App Platform app
`nanolathe-relay`, region `sfo`). `.do/app.yaml` is its exact spec; keep the
two in step by applying the file rather than editing settings in the console:

```sh
doctl apps list                                  # find the app ID
doctl apps update APP-ID --spec .do/app.yaml     # spec changes redeploy
```

- **One always-running instance** of 1 shared vCPU / 512 MiB
  (`apps-s-1vcpu-0.5gb`), with `GOMEMLIMIT=384MiB`. This is not a measured
  capacity claim. Check the displayed bill before changing the size:
  [current pricing](https://docs.digitalocean.com/products/app-platform/details/pricing/).
- **Autodeploy is off.** Pushing a reviewed snapshot does not deploy it. Deploy
  between play tests with `doctl apps create-deployment APP-ID`, then confirm
  `curl https://relay.nanolathe.gg/healthz` returns `ok`.
- **The platform health check uses port 8081**, the image's separate health
  listener. Connections that fill the relay's connection slots on port 8080 cannot
  fail it and restart the instance. Port 8080 keeps its own `/healthz` for
  people checking the public URL.
- The custom domain needs both its DNS CNAME and the domain entry in the spec
  before App Platform issues its certificate. No certificates or secrets
  belong in this repository.

**Keep instance count at 1.** Rooms live in this process's memory; a second
instance could receive a join for a room on the first. A restart or deployment
ends active matches. Deploy between play tests. There is no persistent disk.

## Connect the local game

Both players need the same stamped client build and retail content. Create a
room:

```sh
./nanolathe --root ~/TotalAnnihilation --mod none --map 'ashap plateau' --fullscreen=false --relay-address wss://relay.nanolathe.gg/relay
```

This creates the room and prints its ten-character code in the terminal and game
message ring. Start the second client within two minutes, adding its room code:

```sh
./nanolathe --root ~/TotalAnnihilation --mod none --map 'ashap plateau' --fullscreen=false --relay-address wss://relay.nanolathe.gg/relay --relay-room ABCDEFGHJK
```

Both clients may run on one Mac: their connections still travel through the real
cloud relay. The game starts when both are ready. No `--relay-insecure-loopback`
or custom CA is needed with the managed HTTPS domain. A client that hears
nothing from the relay for 25 seconds during a match stops with a message.

## Local container check

```sh
docker build --platform linux/amd64 -t nanolathe-relay .
docker run --rm --read-only --cap-drop=ALL --security-opt=no-new-privileges -p 127.0.0.1:8080:8080 -p 127.0.0.1:8081:8081 nanolathe-relay
curl --fail http://127.0.0.1:8080/healthz
curl --fail http://127.0.0.1:8081/healthz
```

For this local test only, the game uses `--relay-address ws://127.0.0.1:8080/relay
--relay-insecure-loopback`. Production uses `wss://`. The Docker command's
`--behind-tls-proxy` explicitly permits HTTP inside the platform; do not expose
that listener directly to the public Internet without an HTTPS proxy.

The final image is a static non-root Go executable on scratch. Its default
command serves 128 rooms and 512 connections with `GOMEMLIMIT=384MiB` for the
512 MiB instance. Each room's bytes are bounded (DESIGN_MULTIPLAYER §16.5.1),
so a misbehaving client fails only its own room. Raise `--max-rooms` only with
more memory. Capacity/load tests are separate from the functional tests here.

## Updating the snapshot

UPSTREAM.json records the engine source revision, original and exported hashes.
All engine changes remain in the multiplayer worktree until play testing is
accepted. Export a new **empty** staging directory from a reviewed commit:

```sh
# In the Nanolathe worktree:
tools/export-relay /path/to/empty/staging --revision COMMIT
```

Review the exported files before updating this private repository. The exporter
copies only the relay, wire helpers, command and deployment templates; rewrites
the Go module prefix; and introduces no dependencies. Keep protocol development
upstream to avoid drift. CI runs tests, race detection, and container health.

References: [DigitalOcean WebSocket sample](https://github.com/digitalocean/sample-websocket),
[App spec](https://docs.digitalocean.com/products/app-platform/reference/app-spec/),
[platform limits](https://docs.digitalocean.com/products/app-platform/details/limits/).
