# Run with `make demo`: the paths are relative to the root of the repository, and logpretty is
# taken from PATH, where the target puts bin/ first.
Output assets/demo.gif

Require logpretty

Set Shell bash
Set FontSize 16
Set Width 1000
Set Height 480
Set Padding 20
Set TypingSpeed 35ms

# a NO_COLOR left in the environment would record the output without its colors
Hide
Type "cd assets && unset NO_COLOR && clear"
Enter
Show

Type "cat demo.log"
Sleep 400ms
Enter
Sleep 2.5s

# --utc keeps the times the same wherever the recording is made
Type "logpretty --utc < demo.log"
Sleep 400ms
Enter
Sleep 3.5s

Type "logpretty --utc --level warn --where 'user_id=42' --fields msg,latency < demo.log"
Sleep 400ms
Enter
Sleep 5s
