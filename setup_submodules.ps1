param(
  [Parameter(Mandatory=$true)][string]$Token
)
$ErrorActionPreference = 'Continue'
$user   = "kunxiong971"
$base   = "e:/github/sub2AI"
$backup = "e:/github/sub2AI/_submod_backup"

$urls = @{
  "gpt_image_playground" = "https://github.com/$user/gpt_image_playground.git"
  "infinite-canvas"      = "https://github.com/$user/infinite-canvas.git"
  "lobe-chat"            = "https://github.com/$user/lobe-chat.git"
  "nextchat"             = "https://github.com/$user/NextChat.git"
  "sub2api"              = "https://github.com/$user/sub2api.git"
}

Write-Output "==> Step 1: move existing repos to backup"
if (-not (Test-Path $backup)) { New-Item -ItemType Directory -Path $backup | Out-Null }
foreach ($d in $urls.Keys) {
  if (Test-Path "$base/$d") {
    Move-Item -Path "$base/$d" -Destination "$backup/$d" -Force
    Write-Output "moved $d"
  }
}

Write-Output "==> Step 2: init container repo at $base"
Set-Location $base
if (-not (Test-Path ".git")) {
  git init
  git checkout -b main
}
git config user.email "agent@qclaw.local"
git config user.name  "小彤"

Write-Output "==> Step 3: add 5 submodules"
foreach ($d in $urls.Keys) {
  if (Test-Path "$base/$d") {
    Write-Output "skip $d (already present)"
    continue
  }
  Write-Output "adding $d ..."
  git submodule add $urls[$d] $d
}

Write-Output "==> Step 4: pin lobe-chat to canary"
Set-Location "$base/lobe-chat"
git fetch origin canary
git checkout canary
Set-Location $base
git add lobe-chat

Write-Output "==> Step 5: commit container"
git add -A
git commit -q -m "chore: manage 5 projects as git submodules" 2>&1 | ForEach-Object { $_.ToString().Trim() }
Write-Output "submodule status:"
git submodule status

Write-Output "==> Step 6: push container to kunxiong971/sub2AI"
$pushUrl = "https://$Token@github.com/$user/sub2AI.git"
git remote remove origin -ErrorAction SilentlyContinue
git remote add origin $pushUrl
git push -u origin main 2>&1 | ForEach-Object { $_.ToString().Trim() }

# strip token from local remote url
git remote set-url origin "https://github.com/$user/sub2AI.git"

Write-Output "DONE. Backup of old dirs is at $backup (safe to delete later)."
