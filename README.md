 # AgencyOS: A Computational Environment for AI Agents

AgencyOS is not an operating system for computer users, but rather a computational environment designed specifically for AI agents running on computing infrastructure. It's a Golang library and small wrapper around it - the server binary. The server acts as an orchestration platform providing agents with high-performance cached tools API, state of the art compute router, which supports routing requests to remote cloud GPUs, local GPUs, or even customer's own remote hardware.

## Features

- High-performance multi-agent execution environment for mixed compute
- A set of tools to facilitate Monte-Carlo search in semantic spaces with real-world grounding, for laptops

## Notes on special features

- Automatic requests batching already hear, now with latency of 50ms, but we'll make this a tunable;
- Yes, there is a way to automatically discover maximum batch size for given model, but it would require a benchmarking suite start-up mode - quite easy, but not a priority.

### Coming Soon:

- Plugins to support automated or user-controlled rent of servers
- Internal vector storage and RAG APIs
- Console user interface
- Image support (yes, even in console...)

Combined with LLM request caching, tracking, tagging, tracing, AgencyOS offers a powerful computational environment for AI agents.

## Table of Contents

1. [Introduction](#introduction)
2. [Getting Started](#getting-started)
3. [Contributing](#contributing)
4. [FAQ](#faq)
5. [License](#license)

<a name="introduction"></a>
## Introduction

AgencyOS is a computational environment designed to support AI agents in their tasks by providing them with high-performance tools and compute resources. It's built on top of Golang library and server binary, offering an orchestration platform for efficient execution of agent tasks.

<a name="getting-started"></a>
## Getting Started

Soon!

<a name="contributing"></a>
## Configuration file

Example configuration file:

```yaml
database:
  type: mysql
  host: localhost
  port: 3306

tools:
  serp-api:
    token: ${SERP_API_TOKEN}
  proxy-crawl:
    token: ${PROXY_CRAWL_TOKEN}

compute:
  - endpoint: http://localhost:8001/v1/completions
    type: http-openai
    max-batch-size: 128 # in case of Mistral-7B and A6000 GPU, 48G
```

These days you'll have to copy it to `config.yaml` and fill to your best knowledge, later we might have some basic discovery for M1/M2/M3 Macs and GPU workstations.

## Contributing

We welcome contributions from the community to improve and expand AgencyOS. If you're interested in contributing, please follow these steps:

1. Fork the repository on GitHub.
2. Create a new branch for your changes.
3. Make your modifications and ensure they pass all tests.
4. Submit a pull request describing your changes.

<a name="faq"></a>
## FAQ

### How does AgencyOS differ from traditional operating systems?

AgencyOS is not an operating system for computer users but rather a computational environment designed specifically for AI agents running on computing infrastructure. It provides high-performance tools and compute resources to support agent tasks.

### What are the benefits of using AgencyOS?

AgencyOS offers several benefits, including:

- High-performance multi-agent execution environment for mixed compute
- A set of tools to facilitate Monte-Carlo search in semantic spaces with real-world grounding, for laptops
- Compatibility with various compute resources (cloud GPUs, local GPUs, remote hardware)

### How can I stay updated on AgencyOS developments?

You can follow the project's GitHub repository and join our community discussions to stay informed about new features, improvements, and updates.

<a name="license"></a>
## License

AgencyOS is released under the [MIT License](https://opensource.org/licenses/MIT).

---

That's it! This README.md provides an overview of AgencyOS, its features, and how to contribute. We look forward to your feedback and contributions to make this computational environment even better for AI agents.
