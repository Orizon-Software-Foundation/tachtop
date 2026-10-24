# tachtop

A fast, friendly terminal system monitor for Linux.

`tachtop` shows what your machine is doing right now — CPU, memory, GPUs,
network, disk and processes — in one clear, responsive screen. It is a single
static binary with no daemon, no root requirement and no desktop-specific
dependencies. It works in any terminal emulator.

```
▚ tachtop  my-laptop  · up 2h 5m · load 1.54 2.69 3.14                Sat 21:18
╭─ CPU ─────────────────────────────────────────────╮╭─ MEMORY ─────────────────╮
│12 cores · 2.0 GHz · 61°C                          ││[███████████▊░░░░░░░░] 47%│
│[██▏░░░░░░░░░░░░░░░] 12.0%   Intel(R) Core 5 210H  ││Used 7.2 GiB / 15.2 GiB   │
│⣿⣿⣿⣿⣷⣶⣶⣶⣤⣤⣤⣄⣀⣀⣀                               ││Swap [▋░░░░░░░░░] 977 MiB │
│ 0 █▎░░░░░░░  14%   1 █▏░░░░░░░░  12%   2 █▎░░░░░  ││Available 8.1 GiB · Free  │
╰────────────────────────────────────────────────────╯╰──────────────────────────╯
```

On machines with GPUs, a one-line-per-adapter strip appears between the main
panels. It is hidden when no adapter is detected.

## Features

- **CPU** — total usage, per-core gauges, frequency, temperature and a live
  history graph.
- **Memory** — used, available, free, cache and swap at a glance.
- **GPUs** — every detected DRM adapter, with utilization, temperature, core
  clock, VRAM and power wherever the vendor exposes them.
- **Network** — download and upload rates plus a graph, per interface.
- **Disk** — read/write activity and usage for mounted filesystems.
- **Processes** — CPU, memory, user, state and RSS; sort, filter, inspect and
  terminate processes without leaving the screen.
- **Responsive layout** — panels rearrange automatically for wide, medium and
  narrow terminals.
- **Readable everywhere** — fixed mid-tone colours for accents and graphs with
  terminal-default body text, so both dark and light themes stay legible.
- **No configuration needed** — optional JSON config for theme, refresh rate
  and temperature unit.

## Requirements

- Linux (data comes from `/proc` and `/sys`).
- A terminal of at least **64×22** cells.
- Go **1.27.1 or newer** to build from source.

The binary itself has no runtime dependencies. The compiled binary is
statically linked and can be copied anywhere. NVIDIA statistics use the
driver's `nvidia-smi` when it is installed; without it the GPU is still listed,
but its live values may be unavailable.

## Install

### Build from source

```sh
git clone
cd tachtop
make
./tachtop
```

Or with plain Go:

```sh
CGO_ENABLED=0 go build -o tachtop ./cmd/tachtop
```

Install system-wide:

```sh
sudo make install            # /usr/local/bin/tachtop
sudo make install-desktop    # application menu entry
```

`make install` honours `PREFIX` and `DESTDIR`, for example:

```sh
make install PREFIX="$HOME/.local"
make install PREFIX=/usr DESTDIR=/tmp/pkg
```

### Desktop launcher

Many desktop environments only launch applications from a menu. After
`make install-desktop`, search for **tachtop** in your application menu. The
entry opens tachtop in your default terminal emulator. This works across
desktops because it relies only on the standard `.desktop` and `Terminal=true`
conventions.

## Usage

```sh
tachtop
```

That is all you need. Press `?` inside tachtop for the full key list.

| Key | Action |
| --- | --- |
| `q`, `ctrl+c` | quit |
| `?` | open or close help |
| `p` | pause / resume updates |
| `↑` `↓`, mouse wheel | select a process |
| `PgUp` `PgDn` | move one page |
| `Home` `End` | jump to first / last process |
| `/` | filter by name, user or PID |
| `enter` | show process details |
| `c`, `esc` | clear the filter |
| `s` | cycle sort: CPU → memory → PID → name |
| `r` | reverse the sort order |
| `k` | terminate the selected process |

When you press `k`, tachtop asks first: `y` sends `SIGTERM`, `Y` sends
`SIGKILL` and `esc` cancels. tachtop refuses to signal itself.

### Command-line flags

```
--theme NAME       colour theme (default: from config, or tachtop)
--interval DURATION  refresh interval, e.g. 1s or 1500ms (minimum 250ms)
--config PATH      config file to use
--list-themes      list built-in themes and exit
--version          print the version and exit
--help             show usage
```

## Configuration

Configuration is optional. Without a config file, tachtop uses sensible
defaults. To customise it, create
`~/.config/tachtop/config.json`:

```json
{
  "theme": "tachtop",
  "refresh_ms": 1500,
  "temp_unit": "c"
}
```

| Field | Values | Default |
| --- | --- | --- |
| `theme` | `tachtop`, `dracula`, `gruvbox`, `nord`, `light` | `tachtop` |
| `refresh_ms` | 250–60000 | 1500 |
| `temp_unit` | `c` or `f` | `c` |

The file is read from `$XDG_CONFIG_HOME/tachtop/config.json` (usually
`~/.config/tachtop/config.json`). Override the location with the
`TACHTOP_CONFIG` environment variable or `--config`.

### Themes

| Theme | Description |
| --- | --- |
| `tachtop` | teal and violet (default) |
| `dracula` | the classic dark palette |
| `gruvbox` | warm, earthy colours |
| `nord` | cool arctic blues |
| `light` | darker accents for light terminal backgrounds |

Body text is deliberately left at your terminal's default foreground colour;
the theme only supplies accents, borders and graph colours. This keeps
tachtop readable no matter which terminal profile you use.

Run `tachtop --list-themes` to see the list, or `tachtop --theme nord` to try
one for a single session.

## How it works

tachtop reads Linux's `/proc` and `/sys` filesystems directly for the live
parts of the screen. [gopsutil](https://github.com/shirou/gopsutil) supplies
CPU model and temperature information, filesystem usage and on-demand process
details.

- No background service and no state files.
- No root privileges; some process details (for example another user's
  command line or working directory) are hidden by the kernel and shown as
  `–`.
- Per-process CPU is calculated from `/proc/<pid>/stat` deltas between
  refreshes, so the process table stays fast even with hundreds of processes.
- If a metric is unavailable (common in containers or virtual machines), the
  rest of the screen keeps working.

### GPU support

tachtop lists every DRM adapter in `/sys/class/drm`, so hybrid laptops show
their integrated and discrete GPUs together. The panel appears only when at
least one GPU is detected.

| Vendor | Source | Typical values |
| --- | --- | --- |
| NVIDIA | `nvidia-smi` | utilization, temperature, core clock, VRAM, power |
| AMD | `amdgpu` sysfs and hwmon | utilization, core clock, VRAM, temperature, power |
| Intel | `i915`/`xe` sysfs and hwmon | core clock, temperature when a sensor exists |

Adapter names come from `lspci` when it is installed; otherwise tachtop uses a
generic vendor name. Missing values are omitted rather than displayed as zero. Linux does not expose
shader/CUDA core **counts** through `/sys`; exact NVIDIA counts require NVML,
which would force a dynamically linked binary. tachtop deliberately remains a
single static binary, so it shows core **clock speed** instead of a core count.
If no GPU is listed, the kernel has no DRM card for it (common in VMs and
containers).

## Troubleshooting

**"This terminal is too small"** — make the window at least 64 columns by 22
rows. tachtop rearranges panels as the window grows.

**Temperature shows `–`** — no readable sensor was found in `/sys`. This is
normal on many virtual machines and in containers.

**A GPU is missing from the panel** — only adapters with a DRM card are shown.
Some virtual GPUs, remote sessions and older drivers do not create one.

**An Intel or AMD GPU has no utilization or temperature** — those values are
not exposed by every driver. Frequency and the other available fields still
update.

**Some processes show `–` for details** — the kernel hides command lines,
executables or working directories belonging to other users. Run tachtop as
that user if you need those fields.

**Graphs look like boxes** — your terminal font is missing braille glyphs.
Install a font with Unicode braille support (most programming and terminal
fonts include them).

**Colours look wrong** — tachtop uses 24-bit colour when the terminal reports
support. `TERM=xterm-256color` is a safe fallback. Try the `light` theme on a
light background.

## Development

```sh
make            # build ./tachtop
make run        # build and run
make test       # go test ./...
make vet        # go vet ./...
make fmt        # gofmt
```

Project layout:

```
cmd/tachtop/         CLI, flags and program startup
internal/config/     optional JSON configuration
internal/format/     byte, rate, duration and text helpers
internal/metrics/    /proc, /sys and GPU collection
internal/theme/      colour palettes and blending
internal/ui/         Bubble Tea model, layout, panels and widgets
contrib/             desktop entry
```

tachtop is built with [Bubble Tea](https://github.com/charmbracelet/bubbletea)
and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

## License

GPL-3.0. See [LICENSE](LICENSE).
