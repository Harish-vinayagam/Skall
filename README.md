<div align="center">

# SKALL

**A decentralized, peer-to-peer terminal messenger built in Go — for Linux.**

[![Status](https://img.shields.io/badge/status-in%20development-orange?style=flat-square)](https://github.com/harisshhhhh/skall)
[![License](https://img.shields.io/badge/license-MIT-blue?style=flat-square)](./LICENSE)
[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat-square&logo=go)](https://go.dev/)
[![Platform](https://img.shields.io/badge/platform-Linux-FCC624?style=flat-square&logo=linux&logoColor=black)](https://kernel.org/)

</div>

---

SKALL is a Linux-first terminal messaging application written in Go. It is designed from the ground up as a decentralized, peer-to-peer system — no central servers, no accounts, no cloud dependency.

The project starts with simple TCP-based communication and evolves incrementally toward a libp2p-powered network with a rich, interactive terminal UI built using [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

> **SKALL is an active, incremental learning and portfolio project.** It is honest about its current state. Planned features are clearly marked as planned, not yet implemented.

---

## Why SKALL?

Modern messaging is dominated by centralized platforms. SKALL explores a different direction: what does it look like to build a messaging system where peers talk directly to each other, identity is local, and no single entity controls the network?

This project is a practical study in:

- **Peer-to-peer networking** — how peers discover and connect to each other without a central broker
- **Distributed systems** — how state, coordination, and messaging work across a decentralized network
- **Secure communication** — how to design transport and message security without rolling custom cryptography
- **Go concurrency** — real-world use of goroutines, channels, and concurrent network handling
- **Terminal application development** — building rich, interactive TUI experiences in the terminal

---

## Technology Stack

| Area | Technology |
|---|---|
| Language | Go 1.21+ |
| P2P Networking | [go-libp2p](https://github.com/libp2p/go-libp2p) |
| Terminal UI | [Bubble Tea](https://github.com/charmbracelet/bubbletea) |
| Terminal Styling | [Lip Gloss](https://github.com/charmbracelet/lipgloss) |
| Local Storage | SQLite |
| Serialization | JSON |
| Target Platform | Linux |

---

## Architecture

SKALL is developed in three phases, each building on the last.

### Phase 1 — Foundation (TCP Chat)

```
Terminal A
     │
     │  TCP
     ▼
Terminal B
```

Two terminal instances talk directly over TCP. This is the learning foundation: networking primitives, Go concurrency, and basic message exchange.

### Phase 2 — Structured Messaging

```
  Peer A ──┐
  Peer B ──┼──► Message Router ──► Local History
  Peer C ──┘
```

Multiple simultaneous peer connections with a structured message protocol, routing logic, peer identity, and local message persistence.

### Phase 3 — Decentralized Network (Target Architecture)

```
         SKALL Node
              │
          go-libp2p
         /    │    \
     Peer A  Peer B  Peer C
         \    │    /
          Peer D
```

A fully decentralized peer-to-peer network using libp2p for transport, peer discovery (mDNS / DHT), and NAT traversal. The final target architecture for V1.

### Conceptual V1 System

```
          ┌────────────────────────┐
          │       Terminal UI      │  ← Bubble Tea + Lip Gloss
          └───────────┬────────────┘
                      │
          ┌───────────▼────────────┐
          │    Application Layer   │
          └──┬──────────┬──────────┘
             │          │
     ┌───────▼──┐  ┌────▼────────┐
     │  Messaging│  │   Storage   │  ← SQLite
     │  (Chat +  │  │  (History + │
     │  Groups)  │  │   Peers)    │
     └───────┬───┘  └─────────────┘
             │
     ┌───────▼───────────────────┐
     │        go-libp2p          │  ← Transport, Discovery, Identity
     └───────────────────────────┘
```

---

## Development Roadmap

```
[x] Project planning and architecture design
[x] Technology stack selection
[ ] Phase 1 — Basic TCP terminal chat in Go
[ ] Multiple simultaneous peer connections
[ ] Message protocol design (JSON)
[ ] Peer identity and local key management
[ ] Direct (1-to-1) messaging
[ ] Group messaging
[ ] Local message history (SQLite)
[ ] Peer discovery (mDNS / DHT)
[ ] go-libp2p integration (Phase 3)
[ ] Bubble Tea terminal UI
[ ] Lip Gloss styling and theming
[ ] Testing suite
[ ] Documentation
[ ] V1 release
```

---

## Project Structure

```
Skall/
├── cmd/
│   └── skall/          # Main application entrypoint
├── internal/
│   ├── chat/           # Chat and messaging logic
│   ├── net/            # Network layer (TCP / libp2p)
│   ├── ui/             # Bubble Tea terminal UI
│   └── storage/        # SQLite persistence
├── pkg/
│   ├── peer/           # Peer identity and management
│   ├── message/        # Message types and protocol
│   └── config/         # Configuration
├── docs/               # Documentation and design notes
├── go.mod
├── go.sum
├── LICENSE
└── README.md
```

> The structure above reflects the planned layout. It will evolve as the implementation grows.

---

## Getting Started

### Prerequisites

- **Go 1.21+** — [Install Go](https://go.dev/doc/install)
- **Linux** — SKALL targets Linux environments
- **Git**

### Clone

```bash
git clone https://github.com/harisshhhhh/skall.git
cd skall
```

### Install Dependencies

```bash
go mod download
```

### Run

> The CLI is not yet stable. As development progresses, the intended entry point will be:

```bash
go run ./cmd/skall
```

---

## Planned Usage

The following illustrates the intended user experience. These are design goals, not yet implemented commands.

```bash
# Start SKALL
skall

# Connect to a peer by address
skall connect /ip4/192.168.1.10/tcp/4001/p2p/12D3KooW...

# Open a chat with a known peer
skall chat <peer-id>
```

**Example terminal session:**

```
╭─────────────────────────────────────────────────╮
│  SKALL  v0.1.0                                  │
│  Peer ID: 12D3KooWAbCdEf...                     │
│  Connected peers: 2                             │
╰─────────────────────────────────────────────────╯

[alice] hey, this is skall
[you]   yeah, no servers needed
[bob]   clean
> _
```

---

## Security

Security is a first-class design concern, even though SKALL is still in early development.

The project intends to:

- Use **established cryptographic primitives** (via libp2p's built-in security protocols like Noise or TLS 1.3) rather than implementing custom cryptography
- Establish **authenticated peer identity** using local keypairs
- Design toward **end-to-end message confidentiality** as the architecture matures

**Important distinctions:**

- Secure transport ≠ end-to-end message encryption. Both are necessary for a genuinely private messenger.
- SKALL does **not** currently claim production-grade security. It should be treated as **educational and experimental** until security guarantees are formally implemented and verified.

---

## Current Limitations

SKALL is early-stage. Known limitations include:

- No stable CLI or release build yet
- NAT traversal and firewall compatibility not yet implemented
- No offline message delivery in initial versions
- No mobile or web client (terminal-only, Linux-first)
- No production-grade security guarantees at this stage
- Limited test coverage during early development phases

---

## Contributing

Contributions are welcome. SKALL is best suited for contributors interested in:

- Go networking and systems programming
- P2P / distributed systems research and design
- Terminal UI development (Bubble Tea, Lip Gloss)
- Protocol design and message serialization
- Architecture discussions and documentation

### How to contribute

1. Fork the repository
2. Create a branch (`feature/your-topic` or `fix/issue-name`)
3. Make changes with clear documentation and rationale
4. Open a pull request describing the design intent and current status

Please keep contributions honest and incremental. Reflect the current maturity of the project rather than assuming a production-ready system.

---

## License

SKALL is released under the **MIT License**.

```
MIT License

Copyright (c) 2026 SKALL contributors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

---

<div align="center">
Built with Go · Runs in your terminal · No servers required
</div>
