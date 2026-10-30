# wtfmem

Native Linux memory dashboard, written in Go. Reads `/proc` and requires no elevated privileges; process details unavailable due to permissions are skipped.

## AI authorship

This project was written with AI assistance by OpenAI Codex using **GPT-6 Luna**. It has not necessarily been thoroughly reviewed or security-audited by a human. Treat its output as an aid, not an authoritative diagnosis of system memory use.

## Caveat emptor

This software is provided for informational purposes and may contain bugs, incorrect assumptions, or incomplete memory accounting. Linux memory counters have caveats; process visibility depends on permissions; GPU memory may not be included. Verify important findings independently. Use at your own risk; see the MIT license below for warranty and liability terms.

## Run

Use the included executable (`./wtfmem`), or build from source:

```sh
go build -o wtfmem .
./wtfmem
```

Keys: ↑/↓ or j/k move; Enter expands/collapses a group; `/` searches; Esc clears/closes; `?` help; `r` refresh; `q` quit.

Process PSS apportions shared pages; RSS overlaps. VIRT is address space, not physical RAM. Kernel memory categories and process metrics overlap, so don't add them together. GPU memory may not be reported.

## License

MIT; see [LICENSE](LICENSE).
