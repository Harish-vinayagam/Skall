# SKALL

A decentralized terminal messaging application for Linux.

> Main tech stack: Go, libp2p, Bubble Tea, Lip Gloss, SQLite, JSON, Linux

## Project status

🚧 SKALL is currently under active development.

This project is intentionally being built incrementally. The first implementation will focus on a simple terminal-to-terminal TCP chat in Go, using the standard library for networking. That foundation is a learning step and not the final decentralized architecture. The long-term direction is a peer-to-peer messaging system that can evolve toward libp2p-based networking and a richer terminal experience.

## Overview

SKALL is a Linux-first terminal messaging application being developed in Go. The goal is to explore how decentralized messaging systems can be built from the ground up, starting with the fundamentals of networking, identity, messaging, and terminal interfaces.

This project is primarily a learning and portfolio project focused on:

- Computer networking
- Peer-to-peer networking
- Distributed systems
- Concurrent programming
- Secure communication
- Go
- Linux
- Terminal application development

SKALL is not presented as a production-ready decentralized messenger. It is a deliberately honest, incremental project that begins with small, understandable building blocks and grows toward a more advanced architecture over time.

## Why SKALL?

Many modern messaging systems rely on centralized infrastructure. SKALL is an experiment in understanding how a messaging system can be designed around direct peer communication, local state, discovery, and distributed coordination.

The project is intended to help build practical understanding of:

- how terminals and networked programs interact
- how peer identity and direct connections are established
- how messaging protocols evolve from simple to structured designs
- how distributed systems handle discovery, coordination, and state
- how secure communication design changes as systems become more complex

## Key goals

- Build a terminal-first messaging experience on Linux
- Learn Go networking and concurrent design patterns
- Explore peer-to-peer messaging concepts in a practical project
- Progress from basic TCP communication to a more distributed architecture
- Keep development honest, simple, and educational

## Current implementation vs. V1 goals vs. future plans

### Current implementation

At this stage, the project is still early. The first work focuses on learning how a terminal program can connect two endpoints over TCP and exchange messages.

This is not the final architecture and should not be described as a decentralized or production-grade messaging system.

### V1 goals

The initial version is designed to establish the core messaging system foundation. Planned V1 functionality includes:

- Terminal-based messaging
- User and peer identity
- Peer-to-peer communication
- Direct one-to-one messaging
- Group messaging
- Peer discovery
- Local message history
- Interactive terminal UI
- Linux support

### Future plans

The project is expected to evolve beyond the initial TCP-only design. The long-term architecture is intended to move toward a more decentralized model using libp2p for peer-to-peer communication, with a richer terminal UI and persistent local storage.

## V1 features

The following features are planned for V1 and should be treated as development goals rather than implemented claims:

- Basic terminal-to-terminal chat over TCP
- Multiple simultaneous peer connections
- Simple message routing between peers
- Local message logging and history
- Peer identity concepts
- Basic group chat model
- Peer discovery mechanisms
- Improved terminal user experience

The first implementation will begin with a very simple terminal-to-terminal TCP chat in Go using the standard networking library. This is a learning/foundation step only and should not be interpreted as a final or complete decentralized system.

## Architecture

### Phase 1

```text
Terminal A
     │
     │ TCP
     ▼
Terminal B
```

This phase represents the initial learning step: two terminal instances communicating directly over TCP.

### Phase 2

```text
Multiple TCP clients
        │
        ▼
Basic messaging system
```

This phase expands the basic chat model to handle multiple connections and a more structured messaging flow.

### Phase 3

```text
        SKALL
          │
      libp2p
     /      \
  Peer A   Peer B
     \      /
      Peer C
```

This is the intended long-term direction: a network of peers communicating through libp2p-style peer-to-peer connectivity.

### Conceptual V1 architecture

```text
                 SKALL

            Terminal UI
                 │
          Application Layer
                 │
       ┌─────────┼─────────┐
       │         │         │
     Chat      Groups    Storage
       │         │         │
       └─────────┼─────────┘
                 │
             libp2p
                 │
          P2P Network
```

This architecture is intentionally high-level. It reflects the planned direction of the project, not a finalized protocol or implementation design.

## Technology stack

The following technologies are part of the intended project direction:

| Area | Planned technology |
| --- | --- |
| Language | Go |
| P2P networking | go-libp2p |
| Terminal UI | Bubble Tea |
| Terminal styling | Lip Gloss |
| Local database | SQLite |
| Serialization | JSON |
| Target platform | Linux |

These are the planned technologies for the project direction and learning goals. They are not all assumed to be fully integrated at this stage.

## Development roadmap

The roadmap below reflects planned work, not completed milestones.

```text
[x] Project planning

[ ] Basic Go TCP chat
[ ] Multiple simultaneous peers
[ ] Message protocol
[ ] Peer identity
[ ] Direct messaging
[ ] Group messaging
[ ] Peer discovery
[ ] libp2p integration
[ ] SQLite persistence
[ ] Bubble Tea terminal UI
[ ] Testing
[ ] Documentation
[ ] V1 release
```

The project is expected to move through these stages gradually, with the earliest work focused on understanding the network basics before layering in more advanced features.

## Project structure

A conceptual structure for the repository is shown below:

```text
SKALL/
├── README.md
├── cmd/
│   └── skall/
├── internal/
│   ├── net/
│   ├── chat/
│   ├── ui/
│   └── storage/
├── pkg/
│   ├── peer/
│   ├── message/
│   └── config/
├── docs/
├── go.mod
├── go.sum
└── LICENSE
```

This structure may evolve as the project matures. The exact layout will depend on implementation choices as the application grows.

## Getting started

### Prerequisites

- Go 1.21 or newer
- Linux environment
- Basic terminal access
- Git

### Clone the repository

```bash
git clone https://github.com/<your-username>/SKALL.git
cd SKALL
```

### Install dependencies

```bash
go mod download
```

### Run the project

At this stage, there is no stable CLI or release build. The project is in a foundational development phase and commands will be added as the application evolves.

Conceptually, the project may eventually be run as:

```bash
go run ./cmd/skall
```

This command is a planned workflow, not a guarantee of a fully implemented application yet.

## Example usage

The following examples are conceptual and intended to illustrate the expected UX. They are not guaranteed to reflect commands that are already implemented.

### Initial concept

```bash
skall
```

### Example terminal session

```text
SKALL

Peer ID: 12D3KooW...

Connected Peers: 2

> hello
```

These examples communicate the intended user experience but should not be interpreted as completed functionality.

## Security considerations

Security is an important long-term design concern for SKALL, but the project is still in early development and does not yet claim strong production-grade guarantees.

Planned security work will focus on using established cryptographic and networking primitives rather than implementing custom cryptographic logic from scratch. The project intends to follow standard approaches for secure communication where appropriate.

Important distinction:

- Secure transport is not the same as end-to-end message encryption.
- A transport layer may protect connections between peers, but message confidentiality and authenticity still require proper higher-level design.
- The project does not currently claim end-to-end encrypted messaging unless that capability is formally implemented and verified.

At this stage, the project should be treated as educational and experimental, not as a secure communication platform.

## Current limitations

Because SKALL is in early development, it may have limitations such as:

- Internet connectivity requirements for peer communication
- NAT traversal challenges
- Firewall restrictions
- Peer availability and discoverability issues
- No offline message delivery initially
- No production-grade security guarantees
- No mobile or web client
- Linux-first development focus
- Limited testing and operational maturity

SKALL is not designed to compete with mature messaging applications. It is a learning-focused project that evolves as the system becomes more capable.

## Future plans

The long-term vision for SKALL is to grow from a simple Go TCP learning project into a more capable decentralized terminal messaging system.

Planned future directions include:

- improving peer discovery and network topology
- introducing structured message protocols
- supporting direct and group communication more formally
- integrating libp2p for peer-to-peer networking
- adding persistent local storage with SQLite
- refining the terminal UI with Bubble Tea and Lip Gloss
- improving testing, reliability, and documentation
- exploring secure communication design in a principled way

This project is expected to remain a practical learning project even as it grows in capability.

## Learning objectives

SKALL is a portfolio project intended to build practical knowledge in:

- Go networking and concurrency
- distributed system concepts
- peer-to-peer communication
- message serialization and protocol design
- terminal application development
- local persistence strategies
- secure communication architecture
- practical design trade-offs in decentralized systems

The project is meant to be educational first, useful second, and resilient over time through iterative improvement.

## Project philosophy

SKALL is not trying to become a replacement for production messaging applications. It is a deliberately scoped project for learning and experimentation.

Its purpose is to:

> Build a practical understanding of networking and distributed systems by progressively developing a decentralized terminal messaging application from the ground up.

This philosophy emphasizes learning by implementation, honest status reporting, and incremental growth rather than hype or unrealistic claims.

## Contributing

Contributions are welcome as the project evolves.

At this stage, the project is best suited for contributors who are interested in:

- Go development
- networking and protocol design
- terminal UI work
- decentralized systems research
- architecture discussions
- documentation and testing

If you would like to contribute:

1. Fork the repository
2. Create a feature or learning branch
3. Make changes with clear documentation and rationale
4. Open a pull request describing the design intent and current status

Please keep the project honest and incremental. Contributions should reflect the current maturity level of the project rather than assuming a production-ready system.

## License

This project is intended to be licensed under the MIT License.

```text
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

SKALL is a learning project focused on building a decentralized terminal messaging system in Go, starting small and moving toward a more capable peer-to-peer architecture over time.
