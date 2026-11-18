// Purpose: Export the approved SVG to the macOS app icon resources.
// Used by: Maintainers running swift build/branding/generate-icons.swift from the repository root.
// Responsibilities: Render the PNG master and all standard ICNS sizes using macOS AppKit.
// Important: Packaging uses the committed ICNS; regeneration is only needed after a logo edit.
import AppKit

let root = URL(fileURLWithPath: FileManager.default.currentDirectoryPath)
let source = root.appendingPathComponent("build/branding/kubesift.svg")
guard let image = NSImage(contentsOf: source) else { fatalError("Cannot load \(source.path)") }
let temporary = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
let iconset = temporary.appendingPathComponent("kubesift.iconset")
try FileManager.default.createDirectory(at: iconset, withIntermediateDirectories: true)
defer { try? FileManager.default.removeItem(at: temporary) }

for size in [16, 32, 128, 256, 512] {
    for scale in [1, 2] {
        let pixels = size * scale
        guard let bitmap = NSBitmapImageRep(
            bitmapDataPlanes: nil, pixelsWide: pixels, pixelsHigh: pixels,
            bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
            colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0
        ), let context = NSGraphicsContext(bitmapImageRep: bitmap) else { fatalError("Cannot allocate icon bitmap") }
        NSGraphicsContext.saveGraphicsState()
        NSGraphicsContext.current = context
        image.draw(in: NSRect(x: 0, y: 0, width: pixels, height: pixels))
        NSGraphicsContext.restoreGraphicsState()
        precondition(bitmap.colorAt(x: 0, y: 0)!.alphaComponent == 0, "Icon corners must be transparent")
        guard let png = bitmap.representation(using: .png, properties: [:]) else { fatalError("Cannot encode PNG") }
        let suffix = scale == 2 ? "@2x" : ""
        try png.write(to: iconset.appendingPathComponent("icon_\(size)x\(size)\(suffix).png"))
        if pixels == 1024 { try png.write(to: root.appendingPathComponent("build/branding/kubesift.png")) }
    }
}
let process = Process()
process.executableURL = URL(fileURLWithPath: "/usr/bin/iconutil")
process.arguments = ["-c", "icns", iconset.path, "-o", root.appendingPathComponent("build/darwin/icons.icns").path]
try process.run()
process.waitUntilExit()
precondition(process.terminationStatus == 0, "ICNS generation failed")
