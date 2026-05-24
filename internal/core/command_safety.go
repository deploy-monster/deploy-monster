package core

import "strings"

// allowedExecCommands are the container commands DeployMonster permits through
// app exec, app commands, terminal, cron, and node-executor surfaces. Keep this
// list conservative; these endpoints execute inside managed containers.
var allowedExecCommands = map[string]struct{}{
	"ls": {}, "ll": {}, "la": {}, "dir": {}, "find": {}, "stat": {},
	"cat": {}, "head": {}, "tail": {}, "grep": {}, "egrep": {}, "awk": {},
	"sed": {}, "cut": {}, "sort": {}, "uniq": {}, "wc": {}, "tr": {},
	"base64": {},

	"pwd": {}, "cd": {}, "echo": {}, "printf": {}, "env": {}, "printenv": {},
	"id": {}, "whoami": {}, "hostname": {}, "uname": {}, "uptime": {},
	"date": {},

	"ps": {}, "top": {}, "htop": {}, "kill": {}, "pkill": {}, "killall": {},
	"sleep": {}, "watch": {},

	"ping": {}, "ping6": {}, "curl": {}, "wget": {}, "nc": {}, "netcat": {},
	"ssh": {}, "scp": {}, "rsync": {}, "dig": {}, "nslookup": {}, "host": {},

	"df": {}, "du": {}, "mount": {}, "umount": {}, "ln": {}, "mkdir": {},
	"touch": {}, "file": {}, "tar": {}, "gzip": {}, "gunzip": {}, "zip": {},
	"unzip": {},

	"apt": {}, "apt-get": {}, "yum": {}, "dnf": {}, "apk": {}, "pacman": {},

	"sh": {}, "bash": {}, "zsh": {}, "fish": {}, "python": {}, "python3": {},
	"node": {}, "ruby": {}, "perl": {}, "php": {}, "lua": {},

	"vi": {}, "vim": {}, "nano": {}, "emacs": {}, "ed": {},

	"true": {}, "false": {}, "yes": {}, "seq": {}, "expr": {}, "test": {},
}

var blockedExecCommandFlags = map[string]map[string]struct{}{
	"sh":      {"-c": {}, "-lc": {}},
	"bash":    {"-c": {}, "-lc": {}},
	"zsh":     {"-c": {}, "-lc": {}},
	"fish":    {"-c": {}},
	"python":  {"-c": {}},
	"python3": {"-c": {}},
	"node":    {"-e": {}, "--eval": {}},
	"ruby":    {"-e": {}},
	"perl":    {"-e": {}},
	"php":     {"-r": {}, "-B": {}, "-R": {}, "-E": {}},
	"lua":     {"-e": {}},
}

// SplitCommand splits a shell-style command into argv tokens without invoking a
// shell. Quotes group words; shell operators are left as data for policy checks.
func SplitCommand(cmd string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := rune(0)
	for i := 0; i < len(cmd); i++ {
		ch := rune(cmd[i])
		if inQuote != 0 {
			if ch == inQuote {
				inQuote = 0
			} else {
				current.WriteRune(ch)
			}
			continue
		}
		switch ch {
		case '\'', '"':
			inQuote = ch
		case ' ', '\t', '\n', '\r':
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(ch)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	if len(tokens) == 0 {
		return []string{"/bin/true"}
	}
	return tokens
}

// CommandSafe validates a shell-style command string against the shared exec policy.
func CommandSafe(cmd string) bool {
	return CommandTokensSafe(SplitCommand(cmd))
}

// CommandTokensSafe validates a direct argv command against the shared exec policy.
func CommandTokensSafe(tokens []string) bool {
	if len(tokens) == 0 {
		return false
	}
	base := tokens[0]
	if idx := strings.LastIndex(base, "/"); idx >= 0 {
		base = base[idx+1:]
	}
	base = strings.ToLower(base)
	if _, ok := allowedExecCommands[base]; !ok {
		return false
	}
	if flags, ok := blockedExecCommandFlags[base]; ok {
		for _, arg := range tokens[1:] {
			if _, blocked := flags[arg]; blocked {
				return false
			}
		}
	}
	cmdLower := strings.ToLower(strings.Join(tokens, " "))
	for _, blocked := range []string{"&&", "||", "|", ";", "$(", "`"} {
		if strings.Contains(cmdLower, blocked) {
			return false
		}
	}
	return true
}
