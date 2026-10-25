## Keys and usage:

| Key | Action |
| --- | --- |
| `q`, `ctrl+c` | quit |
| `?` | open or close help |
| `p` | pause / resume updates |
| `↑` `↓`, mouse wheel, click | select a process |
| `PgUp` `PgDn` | move one page |
| `Home` `End` | jump to first / last process |
| `/` | filter by name, user or PID |
| `enter` | show process details (`↑` `↓`, `PgUp` `PgDn` scroll) |
| `c`, `esc` | clear the filter |
| `s` | cycle sort: CPU → memory → PID → name |
| `r` | reverse the sort order |
| `k` | terminate the selected process |

When `k` is pressed, tachtop asks first: `y` sends `SIGTERM`, `Y` sends
`SIGKILL` and `esc` cancels. tachtop refuses to signal itself.

Command line flags:

```
--theme NAME       color theme (default: from config, or tachtop)
--interval DURATION  refresh interval, e.g. 1s or 1500ms (minimum 250ms)
--config PATH      config file to use
--list-themes      list built-in themes and exit
--version          print the version and exit
--help             show usage
```



## Configuration

Configuration is optional. Without a config file, tachtop uses sensible
defaults. To customize it, create
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
`TACHTOP_CONFIG` environment variable or `--config`. Unknown fields are
rejected, so a typo produces a warning instead of being silently ignored.

### Themes

| Theme | Description |
| --- | --- |
| `tachtop` | teal and warm accents (default) |
| `dracula` | the classic dark palette |
| `gruvbox` | warm, earthy colors |
| `nord` | cool arctic blues |
| `light` | darker accents for light terminal backgrounds |

Body text is deliberately left at your terminal's default foreground color;
the theme only supplies accents, borders and graph colors. This keeps
tachtop readable no matter which terminal profile you use.

Run `tachtop --list-themes` to see the list, or `tachtop --theme nord` to try
one for a single session.

## How it works

tachtop reads Linux's `/proc` and `/sys` filesystems directly for the live
parts of the screen. [gopsutil](https://github.com/shirou/gopsutil) supplies
CPU model and temperature information, filesystem usage and on-demand process
details.

- No background service and no state files.
- No root privileges are required. Some process details (for example another
  user's command line or working directory) may be hidden by the kernel and
  shown as `–`; CPU power counters are often restricted to root as well.
- Per-process CPU is calculated from `/proc/<pid>/stat` deltas between
  refreshes, so the process table stays fast even with hundreds of processes.
- CPU package power is calculated from RAPL energy counters in
  `/sys/class/powercap`, with a hwmon fallback for AMD drivers. Some kernels
  only allow root to read those counters; the rest of the panel still works.
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

**"This terminal is too small"**: make the window at least 64 columns by 22
rows. tachtop rearranges panels as the window grows.

**Temperature shows `–`**: no readable sensor was found in `/sys`. This is
normal on many virtual machines and in containers.

**CPU power is missing**: the RAPL energy counters in
`/sys/class/powercap` are often readable only by root. Run tachtop with the
needed permission, or use a kernel and driver that expose CPU power through
hwmon. Everything else keeps working without it.

**A GPU is missing from the panel**: only adapters with a DRM card are shown.
Some virtual GPUs, remote sessions and older drivers do not create one.

**An Intel or AMD GPU has no utilization or temperature**: those values are
not exposed by every driver. Frequency and the other available fields still
update.

**Some processes show `–` for details**: the kernel hides command lines,
executables or working directories belonging to other users. Run tachtop as
that user if you need those fields.

**Graphs look like boxes**: your terminal font is missing braille glyphs.
Install a font with Unicode braille support (most programming and terminal
fonts include them).

**Colours look wrong**: tachtop uses 24-bit colour when the terminal reports
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
