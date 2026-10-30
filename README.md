# wtfmem

Native Linux memory dashboard, written in Go. Reads `/proc` and requires no elevated privileges; process details unavailable due to permissions are skipped.

## AI authorship

This project was written with AI assistance by OpenAI Codex using **GPT-6 Luna**. It has not necessarily been thoroughly reviewed or security-audited by a human.

## Run

Use the included executable (`./wtfmem`), or build from source:

```sh
go build -o wtfmem .
./wtfmem
```

Keys: ↑/↓ or j/k move; Enter expands/collapses a group; `/` searches; Esc clears/closes; `?` help; `r` refresh; `q` quit.

Process PSS apportions shared pages; RSS overlaps. VIRT is address space, not physical RAM. Kernel memory categories and process metrics overlap, so don't add them together. GPU memory may not be reported.

## Releases

GitHub Actions runs tests and a build on pushes and pull requests. Push a version tag (for example, `git tag v0.1.0 && git push origin v0.1.0`) to publish a GitHub Release with Linux amd64 and arm64 binaries, archives, and checksums.

## License

MIT; see [LICENSE](LICENSE).
