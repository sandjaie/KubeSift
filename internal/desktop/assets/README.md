# desktop/assets

`assets.go` embeds the production frontend into the desktop executable. `cmd/kubesift` and the native asset server use the embedded `dist` directory as a filesystem. There is no sidecar web server in the shipped app.

`dist` is Vite output. It is generated and gitignored. Build the frontend before compiling the Go executable, or the embed has nothing to include.
