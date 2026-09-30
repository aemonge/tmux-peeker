# tmux-peeker README demo
#
# Validate without recording:
#   vhs validate demo.tape
#
# Record:
#   vhs demo.tape
#
# If recording is interrupted, clean the isolated server and workspace:
#   root="/tmp/tmux-peeker-vhs-${UID}"
#   tmux -S "$root/tmux.sock" kill-server 2>/dev/null || true
#   rm -rf "$root"

Output assets/demo.gif

Set FontSize 16
Set Width 1040
Set Height 720
Set Padding 12
Set Theme "Gruvbox Light"
Set TypingSpeed 35ms

# Build into an isolated workspace before changing HOME. A private tmux wrapper
# keeps fixture sessions and captured content away from the user's live server.
Hide
Type "set -e"
Enter
Type 'export TMUX_PEEKER_REAL_TMUX=$(command -v tmux)'
Enter
Type 'export TMUX_PEEKER_DEMO_ROOT="/tmp/tmux-peeker-vhs-${UID}"'
Enter
Type 'mkdir "$TMUX_PEEKER_DEMO_ROOT"'
Enter
Type 'export TMUX_PEEKER_DEMO_SOCKET="$TMUX_PEEKER_DEMO_ROOT/tmux.sock"'
Enter
Type "trap '\$TMUX_PEEKER_REAL_TMUX -S \$TMUX_PEEKER_DEMO_SOCKET kill-server >/dev/null 2>&1 || true; rm -rf \$TMUX_PEEKER_DEMO_ROOT' EXIT INT TERM"
Enter
Type 'mkdir -p "$TMUX_PEEKER_DEMO_ROOT/bin" "$TMUX_PEEKER_DEMO_ROOT/home" "$TMUX_PEEKER_DEMO_ROOT/config"'
Enter
Type 'export XDG_CONFIG_HOME="$TMUX_PEEKER_DEMO_ROOT/config"'
Enter
Type 'make build BINARY="$TMUX_PEEKER_DEMO_ROOT/bin/tmux-peeker" >/dev/null'
Enter
Type 'printf "#!/bin/sh\nexec \"\$TMUX_PEEKER_REAL_TMUX\" -S \"\$TMUX_PEEKER_DEMO_SOCKET\" -f /dev/null \"\$@\"\n" > "$TMUX_PEEKER_DEMO_ROOT/bin/tmux"'
Enter
Type 'chmod +x "$TMUX_PEEKER_DEMO_ROOT/bin/tmux"'
Enter
Type 'export PATH="$TMUX_PEEKER_DEMO_ROOT/bin:$PATH"'
Enter
Type 'export HOME="$TMUX_PEEKER_DEMO_ROOT/home" SHELL=/bin/sh PS1="peek> "'
Enter
Type "unset TMUX TMUX_PANE RPROMPT"
Enter
Type "scripts/test-fixture.sh up >/dev/null"
Enter
Type "clear"
Enter
Show
Sleep 250ms

# --- Segment 1: live session previews --------------------------------------
Type "echo '→ tmux-peeker: peek before you switch'"
Enter
Sleep 700ms
Type "tmux-peeker --theme solarized-gruvbox"
Enter
Wait+Screen /tmux sessions \(3\)/
Sleep 1s
Type "q"
Sleep 600ms

# --- Segment 2: filtering ----------------------------------------------------
Type "echo '→ Filter the session list with /'"
Enter
Sleep 700ms
Type "tmux-peeker --theme solarized-gruvbox"
Enter
Wait+Screen /tmux sessions \(3\)/
Sleep 500ms
Type "/"
Sleep 250ms
Type@70ms "multi"
Wait+Screen /> multi/
Sleep 500ms
Enter
Wait+Screen /tmux sessions \(1\).*\/multi/
Sleep 500ms
Type "q"
Sleep 600ms

# --- Segment 3: hierarchy drill ---------------------------------------------
Type "echo '→ Tab drills into windows and panes; the live preview follows the cursor'"
Enter
Sleep 700ms
Type "tmux-peeker --theme solarized-gruvbox"
Enter
Wait+Screen /tmux sessions \(3\)/
Sleep 400ms
Type "/"
Sleep 250ms
Type@70ms "multi"
Wait+Screen /> multi/
Sleep 300ms
Enter
Wait+Screen /tmux sessions \(1\).*\/multi/
Sleep 400ms
Tab
Wait+Screen /tmux window picker/
Sleep 900ms
Down
Sleep 700ms
Tab
Wait+Screen /tmux pane picker/
Sleep 900ms
Down
Sleep 700ms
Type "q"
Sleep 600ms

# --- Segment 4: interactive window deck --------------------------------------
Type "echo '→ Press i to stack a session into live window bands'"
Enter
Sleep 700ms
Type "tmux-peeker --theme solarized-gruvbox"
Enter
Wait+Screen /tmux sessions \(3\)/
Sleep 400ms
Type "/"
Sleep 250ms
Type@70ms "multi"
Wait+Screen /> multi/
Sleep 300ms
Enter
Wait+Screen /tmux sessions \(1\).*\/multi/
Sleep 500ms
# The chevron rules bracket every deck band; the wrap indicator only renders
# when windows fall off-screen, so anchor on the rule shape instead.
Type "i"
Wait+Screen /❯.*›.*❮/
Sleep 1s
Type "j"
Sleep 800ms
Type "j"
Sleep 800ms
Type "k"
Sleep 800ms
Escape
Wait+Screen /tmux sessions \(1\)/
Sleep 600ms
Type "q"
Sleep 600ms

# --- Segment 5: contextual help ---------------------------------------------
Type "echo '→ Press ? for contextual help; q quits'"
Enter
Sleep 700ms
Type "tmux-peeker --theme solarized-gruvbox"
Enter
Wait+Screen /tmux sessions \(3\)/
Sleep 400ms
Type "?"
Wait+Screen /close help/
Sleep 1500ms
Escape
Wait+Screen /tmux sessions \(3\)/
Sleep 500ms
Type "q"
Sleep 400ms

# Remove only the isolated server, fixtures, binary, HOME, and config.
Hide
Type '"$TMUX_PEEKER_REAL_TMUX" -S "$TMUX_PEEKER_DEMO_SOCKET" kill-server >/dev/null 2>&1 || true'
Enter
Type 'rm -rf "$TMUX_PEEKER_DEMO_ROOT"'
Enter
Type "trap - EXIT INT TERM"
Enter
