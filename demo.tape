Output assets/demo.gif
Set FontSize 16
Set Width 1200
Set Height 600
Set Theme "Dracula"
Set TypingSpeed 40ms
Set Padding 20

# Version
Type "kc --version"
Enter
Sleep 1s

# List vaults
Type "kc vault list"
Enter
Sleep 1.5s

# List keys
Type "kc list"
Enter
Sleep 2s

# Search
Type "kc search openai"
Enter
Sleep 2s

# kc run (process-scoped)
Type "kc run -- env | head -5"
Enter
Sleep 2s

# kc inject
Type "kc inject --key TEST_KEY"
Enter
Sleep 1.5s

# Open TUI
Type "kc"
Enter
Sleep 3s

# Navigate
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms

# Search in TUI
Type "/"
Sleep 500ms
Type "api"
Sleep 1s
Escape
Sleep 500ms

# Tab switch vault
Tab
Sleep 1s
Tab
Sleep 1s

# Help overlay
Type "?"
Sleep 2s
Escape
Sleep 500ms

# Quit
Type "q"
Sleep 1s
