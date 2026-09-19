#!/bin/bash
set -euo pipefail

branch="main"
report_only=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --branch)
      shift
      branch="$1"
      ;;
    --report-only)
      report_only=true
      ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
  shift
done

if [[ -z "${SONAR_TOKEN:-}" ]]; then
  echo "SONAR_TOKEN is required" >&2
  exit 1
fi

if [[ -z "${GH_TOKEN:-}" ]]; then
  report_only=true
fi

PROJECT="jwp23_agent-downlink"
HOST="https://sonarcloud.io"

# Fetch open issues
issues_response=$(
  printf 'header = "Authorization: Bearer %s"\n' "$SONAR_TOKEN" | \
  curl -sSf --config - \
    "$HOST/api/issues/search?componentKeys=$PROJECT&branch=$branch&resolved=false&ps=100"
)
open_issues=$(echo "$issues_response" | jq '.total')
issues_lines=$(echo "$issues_response" | jq -r '.issues[] | "\(.severity) \(.rule) \(.component | sub("^[^:]+:"; "")) \(.line // .textRange.startLine) \(.message)"')

# Fetch unreviewed hotspots
hotspots_response=$(
  printf 'header = "Authorization: Bearer %s"\n' "$SONAR_TOKEN" | \
  curl -sSf --config - \
    "$HOST/api/hotspots/search?projectKey=$PROJECT&branch=$branch&status=TO_REVIEW&ps=100"
)
unreviewed_hotspots=$(echo "$hotspots_response" | jq '.paging.total')
hotspots_lines=$(echo "$hotspots_response" | jq -r '.hotspots[] | "\(.ruleKey) \(.component | sub("^[^:]+:"; "")) \(.line) \(.message)"')

# Print summary
total=$((open_issues + unreviewed_hotspots))
echo "branch=$branch open_issues=$open_issues unreviewed_hotspots=$unreviewed_hotspots"

# Print findings
if [[ -n "$issues_lines" ]]; then
  echo "$issues_lines"
fi
if [[ -n "$hotspots_lines" ]]; then
  echo "$hotspots_lines"
fi

# If report-only mode, exit early
if [[ "$report_only" == "true" ]]; then
  exit 0
fi

# Handle GitHub issue
issue_title="SonarCloud drift on main"
issue_link="https://sonarcloud.io/project/issues?id=$PROJECT&resolved=false&branch=$branch"

if [[ $total -eq 0 ]]; then
  # Close the issue if it exists
  issue_number=$(gh issue list --state open --search "in:title \"$issue_title\"" --json number -q '.[0].number // empty' 2>/dev/null || true)
  if [[ -n "$issue_number" ]]; then
    gh issue close "$issue_number" --comment "SonarCloud drift resolved on $branch."
  fi
  exit 0
else
  # Create or update the issue
  issue_body="Drift audit found $open_issues open issues and $unreviewed_hotspots unreviewed hotspots on branch \`$branch\`.

Fixes are tracked in beads; this GitHub issue is transient.

$issue_link
"
  if [[ -n "$issues_lines" ]]; then
    issue_body+=$'\n## Open Issues\n\n'
    issue_body+="$issues_lines"$'\n'
  fi
  if [[ -n "$hotspots_lines" ]]; then
    issue_body+=$'\n## Unreviewed Hotspots\n\n'
    issue_body+="$hotspots_lines"$'\n'
  fi

  issue_number=$(gh issue list --state open --search "in:title \"$issue_title\"" --json number -q '.[0].number // empty' 2>/dev/null || true)
  if [[ -n "$issue_number" ]]; then
    gh issue edit "$issue_number" --body "$issue_body"
  else
    gh issue create --title "$issue_title" --body "$issue_body"
  fi
  exit 1
fi
