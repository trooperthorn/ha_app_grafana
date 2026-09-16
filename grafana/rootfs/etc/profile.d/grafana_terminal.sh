# shellcheck shell=bash
# Grafana (hardened) terminal: history, prompt and helpers for every login bash.

export HISTFILE=/data/terminal/.bash_history
export HISTSIZE=50000
export HISTFILESIZE=50000
export HISTTIMEFORMAT='%Y-%m-%d %H:%M:%S  '
export HISTCONTROL=ignoredups
shopt -s histappend
shopt -s checkwinsize
PROMPT_COMMAND="history -a${PROMPT_COMMAND:+; ${PROMPT_COMMAND}}"

# Idle timeout: a courtesy for a forgotten tab, not a control.
if [ "${GRAFANA_TERM_IDLE_MINUTES:-0}" -gt 0 ] 2>/dev/null; then
    TMOUT=$(( GRAFANA_TERM_IDLE_MINUTES * 60 ))
    export TMOUT
fi

__grafana_prompt() {
    local status=$?
    local red='\[\e[31m\]' green='\[\e[32m\]' blue='\[\e[34m\]' dim='\[\e[2m\]' reset='\[\e[0m\]'
    local mark="${green}\$${reset}"
    [ "${status}" -ne 0 ] && mark="${red}${status} \$${reset}"
    PS1="${dim}grafana${reset} ${blue}\w${reset} ${mark} "
}
PROMPT_COMMAND="__grafana_prompt${PROMPT_COMMAND:+; ${PROMPT_COMMAND}}"

export PATH="/usr/share/grafana/bin:${PATH}"
export GF_PATHS_HOME=/usr/share/grafana
alias ls='ls --color=auto'
alias ll='ls -la --color=auto'
alias grep='grep --color=auto'
export LESS="-R"
export PAGER="less"
# Read-only look at the Grafana database. Grafana holds the write lock.
alias gdb='sqlite3 -readonly /data/grafana/grafana.db'

cd /data 2>/dev/null || true

if [ -n "${GRAFANA_TERM_SESSION_ID:-}" ]; then
    printf 'Grafana terminal %s  session %s  user %s  %s\n' \
        "${GRAFANA_TERM_VERSION:-}" "${GRAFANA_TERM_SESSION_ID}" "${GRAFANA_TERM_USER:-unknown}" \
        "$([ "${GRAFANA_TERM_RECORD:-true}" = "true" ] && echo recorded || echo 'not recorded')"
    printf 'This shell is the grafana user inside this container: /data (Grafana state, plugins, logs, secrets), grafana cli, gdb (read-only sqlite). Nothing beyond this container.\n'
fi
