# helmrender/wire

`wire` is the private protocol between the native Helm renderer and its bundled WebAssembly worker. `protocol.go` defines the messages. `codec.go` keeps Helm scalar types and empty collections intact across that boundary.

Only the bundled worker speaks this protocol. Chart bytes are not decoded as gob. Payloads stay in memory. They are not written to files, logs, or application result history.
