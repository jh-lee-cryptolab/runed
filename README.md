# runed — Shared embedding daemon

`runed` is a Go daemon that wraps [llama.cpp](https://github.com/ggml-org/llama.cpp)
`llama-server` to serve Qwen3-Embedding-0.6B embeddings via gRPC over a UNIX
domain socket. It is designed as a **shared singleton process per machine** so
that multiple client sessions do not each load their own ~400 MB embedding
model.

## Installation

TBD

## Usage

TBD

## Configuration

TBD

## Model variants

The f16 GGUF is the parity reference — cosine similarity ≥ 0.9999 against
sentence-transformers on all 8 fixture texts. Quantized variants trade parity
for size/latency.

| Variant | Size | Mean cosine | Verdict |
|---|---|---|---|
| f16 | 1.1 GB | ≈ 0.99999 | parity reference |
| q8_0 | 610 MB | ≈ 0.9993 | high-fidelity alternative |
| q6_K | 472 MB | 0.994 | **production default** |
| q5_K_M | 424 MB | 0.990 | borderline; natural-language only |
| q4_K_M | 378 MB | 0.971 | rerank-only or dev staging; not sole retrieval backbone |

**Production default is q6_K** (`models/qwen3-embedding-0.6b.q6_K.gguf`) —
23% smaller than q8_0 with mean cosine 0.994 against the f16 reference.
f16 is reserved for parity verification against sentence-transformers.
Set the daemon's `RUNED_MODEL` env var to the desired GGUF path.

## License

- `runed` Go code: MIT (LICENSE file forthcoming).
- Qwen3-Embedding-0.6B: Apache 2.0.
- llama.cpp: MIT.

Redistribution of the bundled `llama-server` binary and GGUF model files is
permitted under the respective upstream licenses; CryptoLab makes no
independent claims on those artifacts.
