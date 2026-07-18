# HWiNFO64 System Summary — Go + Qt recreation

A pixel-oriented recreation of the HWiNFO64 **System Summary** window, built with
**Go** and **Qt 6** via the [`miqt`](https://github.com/mappu/miqt) bindings.

![reference: AMD Ryzen 7 5800X / ASUS ROG STRIX B550-F / RTX 3080]

All values are **static** (taken from the reference screenshot) — this reproduces
the *look* of the window, it is not a live hardware monitor. HWiNFO itself is
Windows-only; this runs on Linux (X11/Wayland).

## Layout

| File        | Purpose                                                        |
|-------------|---------------------------------------------------------------|
| `main.go`   | App entry, frameless window + title bar, the three columns.   |
| `style.go`  | Global stylesheet (dark/teal theme) and widget helpers.       |
| `data.go`   | Static data: feature flags, clocks, timings, drives.          |
| `build.sh`  | Build wrapper that sets up `pkg-config` for Qt6.              |

## Requirements

- Go 1.21+
- A C/C++ compiler (`gcc`/`g++`)
- **Qt 6 development files** (headers, `moc`, pkg-config `.pc` files)

On Fedora:

```sh
sudo dnf install qt6-qtbase-devel
```

(Debian/Ubuntu: `qt6-base-dev`; Arch: `qt6-base`.)

## Build & run

With Qt6 devel installed system-wide, the plain toolchain works:

```sh
go build        # produces ./hwinfo-gui
./hwinfo-gui
```

If Qt6's pkg-config files live in a non-standard location, use the wrapper,
which points `PKG_CONFIG_PATH`/`PATH` at a prefix given by `QT_LOCAL_PREFIX`:

```sh
QT_LOCAL_PREFIX=/path/to/qt6/usr ./build.sh build
QT_LOCAL_PREFIX=/path/to/qt6/usr ./build.sh run
```

## Headless screenshot

Render the window to a PNG without a visible display (uses Qt's offscreen
platform), handy for CI or previews:

```sh
QT_QPA_PLATFORM=offscreen HWINFO_SHOT=out.png ./hwinfo-gui
```

## Notes

- The **AMD Ryzen** and **NVIDIA** logos are lightweight stylized placeholders
  (colored glyphs), not the trademarked artwork.
- The window is frameless with a custom title bar; drag it by the title bar,
  close it with the red ✕.
