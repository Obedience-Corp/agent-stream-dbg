# Home UX prototype

Bubble Tea mock of the **launch home / settings hub** proposed for
`stream-debugger` bare launch. Not the product binary.

Design: `workflow/design/stream-debugger-home-tui/` (campaign).

## Run

```bash
# from repo root
just demo-home-ux

# multi-config seed
SD_HOME_UX_SEED=multi just demo-home-ux
```

## Record VHS

```bash
just record-home-ux
# or: demos/home-ux/record.sh [all|first-run|select-open|new-config]
```

Outputs:

- `docs/assets/home-ux-first-run.gif`
- `docs/assets/home-ux-select-open.gif`
- `docs/assets/home-ux-new-config.gif`

## Keys

| Key | Home | Wizard / Editor |
|-----|------|-----------------|
| `↑↓` `jk` | Move | Move / fields |
| `Enter` | Open / activate | Next / confirm |
| `n` | New config | — |
| `e` | Edit | — |
| `d` | Delete | — |
| `Esc` | — | Back |
| `Ctrl+S` | — | Save (editor) |
| `q` | Quit | — |
