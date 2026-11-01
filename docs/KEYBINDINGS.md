# Keybindings Reference

`lazyssh` is designed for rapid, keyboard-centric operation. Most actions follow familiar Vim and standard terminal utility keybindings.

---

## 1. Global Navigation

These keybindings are active across all panels unless an input modal or search prompt is open.

| Key | Action | Description |
| :--- | :--- | :--- |
| `q` | **Quit** | Exits `lazyssh` cleanly. |
| `Ctrl+C` | **Force Quit** | Immediately halts execution. |
| `?` | **Help** | Toggles the in-app help modal & keybindings cheatsheet. |
| `Tab` | **Next Panel** | Cycles focus clockwise to the next panel. |
| `Shift+Tab` | **Prev Panel** | Cycles focus counter-clockwise to the previous panel. |
| `1` | **Focus Hosts** | Jumps directly to Panel 1 (Hosts Explorer). |
| `2` | **Focus Details** | Jumps directly to Panel 2 (Host Inspector). |
| `3` | **Focus Tunnels** | Jumps directly to Panel 3 (Active Tunnels). |
| `4` | **Focus Keys** | Jumps directly to Panel 4 (SSH Keys & Identities). |
| `/` | **Filter / Search** | Opens the fuzzy search input prompt. |
| `Esc` | **Clear / Back** | Clears the active filter, cancels an action, or closes a modal. |
| `R` | **Reload Config** | Re-parses `~/.ssh/config` from disk immediately. |

---

## 2. Hosts Panel (Panel 1)

Active when the Hosts panel is focused:

| Key | Action | Description |
| :--- | :--- | :--- |
| `j` or `↓` | **Down** | Move selection down by one host. |
| `k` or `↑` | **Up** | Move selection up by one host. |
| `g` or `Home` | **Top** | Jump to the first host in the list. |
| `G` or `End` | **Bottom** | Jump to the last host in the list. |
| `Ctrl+d` | **Half Page Down** | Scroll down half a page. |
| `Ctrl+u` | **Half Page Up** | Scroll up half a page. |
| `Enter` | **Connect** | Suspends TUI and launches interactive SSH session to selected host. |
| `Space` | **Bookmark / Favorite** | Toggles favorite status on the selected host. |
| `a` | **Add Host** | Opens interactive modal form to create a new host block. |
| `e` | **Edit Host** | Opens the host configuration in `$EDITOR` or built-in editor. |
| `d` | **Delete Host** | Deletes the host entry from `~/.ssh/config` (after confirmation). |
| `c` | **Duplicate Host** | Clones selected host into a new entry. |
| `y` | **Copy Command** | Copies `ssh user@hostname -p port` to system clipboard. |
| `p` | **Ping Host** | Probes remote host via TCP dial and displays latency. |
| `P` | **Ping All** | Concurrently probes all visible hosts in the list. |
| `t` | **Manage Tunnels** | Opens tunnel creation/management modal for selected host. |
| `T` | **Add Tag** | Opens tag management prompt for selected host. |

---

## 3. Tunnel Manager Panel (Panel 3)

Active when the Tunnels panel is focused:

| Key | Action | Description |
| :--- | :--- | :--- |
| `j` or `↓` | **Down** | Select next tunnel. |
| `k` or `↑` | **Up** | Select previous tunnel. |
| `Enter` or `s` | **Toggle Tunnel** | Start or stop the selected tunnel. |
| `a` | **New Tunnel** | Open form to define a new Local, Remote, or SOCKS5 tunnel. |
| `d` | **Delete Tunnel** | Remove tunnel definition. |
| `r` | **Restart Tunnel** | Restarts active tunnel process. |
| `l` | **View Tunnel Logs** | Opens log inspector modal for background tunnel process. |

---

## 4. Search & Filter Mode

Activated by pressing `/`:

| Key | Action | Description |
| :--- | :--- | :--- |
| Any text | **Type Query** | Fuzzy filters hosts by alias, hostname, user, or tag. |
| `Enter` | **Apply Filter** | Commits search filter and returns focus to host list. |
| `Esc` | **Cancel Search** | Clears filter and returns to full list. |
| `Ctrl+U` | **Clear Input** | Deletes entire search string. |
| `↓` / `↑` | **Quick Navigate** | Moves selection in filtered list without exiting search mode. |

---

## 5. Modal & Form Inputs

When editing a host or defining a tunnel:

| Key | Action | Description |
| :--- | :--- | :--- |
| `Tab` | **Next Field** | Moves cursor to the next input field. |
| `Shift+Tab` | **Prev Field** | Moves cursor to the previous input field. |
| `Enter` | **Submit** | Validates and saves the form. |
| `Esc` | **Cancel** | Discards changes and closes modal without saving. |
