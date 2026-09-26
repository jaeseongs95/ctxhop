# CtxHop

<p align="center">
  <img src="assets/ctxhop-logo.png" alt="CtxHop 로고" width="180">
</p>

<p align="center">
  <a href="https://github.com/CCCCY-ci/ctxhop/releases/latest"><img src="https://img.shields.io/github/v/release/CCCCY-ci/ctxhop?sort=semver" alt="최신 릴리스"></a>
  <a href="https://github.com/CCCCY-ci/ctxhop/blob/main/LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="MIT 라이선스"></a>
</p>

<p align="center">
  <img src="assets/home.png" alt="CtxHop 설치 완료" width="900">
</p>

[English](README.md) | [简体中文](README.zh-CN.md) | 한국어

**기기를 바꿔도 컨텍스트는 그대로 이어집니다.**

CtxHop은 Claude Code와 Codex 세션을 기기 사이에서 옮기는 로컬 우선 CLI입니다. 프로젝트를 연결하고, 직접 관리하는 스토리지로 세션 기록을 동기화한 뒤, 승인된 어느 기기에서든 세션을 이어 갑니다.

Session Hub는 각 에이전트의 네이티브 세션을 논리 세션으로 묶고 원본 기록을 보존합니다. 고른 컨텍스트로 대상 에이전트의 네이티브 세션을 만들어 에이전트를 바꿀 수도 있습니다. 작업 공간과 Git 상태는 명시적으로 요청할 때만 넘기며, 로컬 암호화와 기기 승인이 동기화 경계를 보호합니다.

## 주요 기능

- **기기 간 세션 이어 가기**: 승인된 다른 기기에서 프로젝트 세션을 이어서 작업합니다.
- **Session Hub**: Claude Code와 Codex 세션을 하나의 논리 세션으로 모으고, 각 네이티브 원본과 기록을 보존합니다.
- **에이전트 전환**: `ctxhop session switch`로 고른 컨텍스트를 다른 에이전트의 새 네이티브 세션으로 옮깁니다.
- **작업 공간 인계**: 필요하면 고른 작업 공간 파일과 Git 상태를 세션과 함께 옮깁니다.
- **로컬 우선 스토리지**: 데이터를 기기에서 암호화한 뒤 직접 관리하는 백엔드에 저장합니다.

## 전체 구조

CtxHop은 단순한 계층 구조를 씁니다.

~~~text
Domain
└── Hub
    └── Project
        └── Session
            ├── Claude Code native Session / Replica
            └── Codex native Session / Replica
~~~

- **도메인**은 암호화된 동기화 경계입니다. 원격 네임스페이스, 키 파일, 승인된 기기가 하나의 공유 데이터 공간을 이룹니다.
- **허브**는 도메인 안의 논리적 프로젝트 공간입니다. 프로젝트를 묶고 서로 분리하며, 새 도메인은 `default` 허브로 시작합니다.
- **프로젝트**는 작업 공간, Git 상태, 세션을 담는 프로젝트 단위의 경계입니다.
- **세션**은 여러 에이전트가 함께 쓰는 논리적 개발 컨텍스트입니다.

평소에는 도메인과 `default` 허브가 겉으로 드러나지 않습니다. 사용자는 현재 프로젝트와 그 세션으로 작업합니다. 같은 승인 도메인 안에서 프로젝트 묶음을 따로 나누고 싶을 때만 다른 허브를 씁니다.

## 데모

![CtxHop 데모](assets/ctxhop.gif)

## 설치

[Releases](https://github.com/CCCCY-ci/ctxhop/releases)에서 운영체제와 CPU 아키텍처에 맞는 패키지를 내려받습니다.

### Windows

CPU 아키텍처에 맞는 설치 프로그램을 내려받아 실행합니다.

- CtxHop-Setup_<version>_windows_amd64.exe
- CtxHop-Setup_<version>_windows_arm64.exe

설치 프로그램은 CtxHop을 `%USERPROFILE%\.ctxhop\bin`에 설치하고, 이 디렉터리를 현재 사용자의 PATH에 추가합니다. 관리자 권한은 필요하지 않습니다. 새 터미널을 열고 설치를 확인합니다.

~~~powershell
ctxhop version
~~~

설치 없이 쓰려면 `ctxhop_<version>_windows_<arch>.zip`을 내려받아 `ctxhop.exe`를 꺼내고, 그 디렉터리를 PATH에 추가합니다.

### macOS / Linux

플랫폼과 CPU 아키텍처에 맞는 압축 파일을 고릅니다.

- macOS Intel: `ctxhop_<version>_darwin_amd64.zip`
- macOS Apple Silicon: `ctxhop_<version>_darwin_arm64.zip`
- Linux x86_64: `ctxhop_<version>_linux_amd64.zip`
- Linux ARM64: `ctxhop_<version>_linux_arm64.zip`

터미널에서 압축을 풀고 설치합니다.

~~~bash
unzip ctxhop_<version>_<os>_<arch>.zip
sh install.sh
~~~

기본 설치 디렉터리는 `$XDG_BIN_HOME`이 설정되어 있으면 그 값이고, 없으면 `$HOME/.local/bin`입니다. 다른 사용자 수준 디렉터리를 쓰려면 `CTXHOP_INSTALL_DIR`을 설정합니다.

~~~bash
CTXHOP_INSTALL_DIR=/path/to/bin sh install.sh
~~~

이 디렉터리가 PATH에 없으면 설치 프로그램이 필요한 셸 설정을 출력합니다. 새 터미널을 열고 설치를 확인합니다.

~~~bash
ctxhop version
~~~

### Go로 설치(선택)

Go 1.26 이상이 필요합니다.

~~~bash
go install github.com/CCCCY-ci/ctxhop/cmd/ctxhop@latest
~~~

Go의 바이너리 디렉터리가 PATH에 있는지 확인합니다. 특정 버전으로 고정해야 하면 `@latest`를 릴리스 태그로 바꿉니다.

### CtxHop 초기화

어느 플랫폼이든 CLI를 설치한 뒤 다음을 실행합니다.

~~~bash
ctxhop init
~~~

이 명령은 스토리지, 암호화, 기기 식별 정보, 에이전트 훅을 설정한 뒤 동기화 도메인을 새로 만들거나 기존 도메인에 참여합니다.

### 제거

~~~bash
ctxhop uninstall
~~~

제거하면 로컬 CLI, 설정, 기기 키, 상태, 로그와 CtxHop이 설치한 에이전트 훅이 삭제됩니다. 원격 객체와 로컬 디렉터리 백엔드의 데이터는 남습니다. 디렉터리 백엔드가 로컬 설정 디렉터리와 겹치면 먼저 백엔드를 옮기세요.

## 빠른 시작: Cloudflare R2

이 빠른 시작은 Cloudflare R2를 사용합니다. 다른 S3 호환 객체 스토리지도 같은 순서를 따릅니다. R2 버킷과 그 Access Key, Secret Access Key를 준비하고, 두 기기 모두에 프로젝트 작업 사본을 준비합니다.

아래 명령은 자동으로 만들어진 `default` 허브를 사용합니다.

R2 설정 예시:

~~~text
Endpoint: https://<ACCOUNT_ID>.r2.cloudflarestorage.com
Bucket:   <BUCKET_NAME>
Region:   auto
Prefix:   ctxhop/demo     # 선택 사항
~~~

하나의 동기화 도메인에 속한 모든 기기에서 같은 버킷과 접두사를 사용합니다.

### 1. 기기 A 초기화

~~~bash
ctxhop init --backend s3 --endpoint "https://<ACCOUNT_ID>.r2.cloudflarestorage.com" --bucket "<BUCKET_NAME>" --region "auto" --prefix "ctxhop/demo" --device-name "device-a"
~~~

안내에 따라 R2 자격 증명과 암호화 비밀번호를 입력합니다. 일반 R2 API 토큰을 쓰면 R2 세션 토큰 입력란은 비워 둡니다. 처음 초기화할 때 만들어지는 **복구 키**(Recovery Key)는 오프라인에 보관합니다.

초기화 중에 에이전트 훅을 설치할지 고릅니다. 훅은 끝난 세션을 자동으로 푸시합니다. 훅을 건너뛰려면 `--no-hook`을 사용합니다.

### 2. 프로젝트 연결과 푸시

기기 A에서 세션을 마친 뒤 프로젝트 디렉터리에서 다음 명령을 실행합니다.

~~~bash
cd /path/to/project
ctxhop project bind --path .
ctxhop push
~~~

커밋하지 않은 작업 공간 파일과 Git 상태까지 포함하려면 다음을 사용합니다.

~~~bash
ctxhop push --workspace
~~~

### 3. 기기 B 승인

기기 A에서 초대 파일을 만듭니다.

~~~bash
ctxhop device invite --output ctxhop-device-b.json
~~~

초대 파일을 기기 B로 옮긴 뒤 다음을 실행합니다.

~~~bash
ctxhop init --invite ./ctxhop-device-b.json --device-name "device-b"
~~~

기기 B의 R2 자격 증명과 같은 암호화 비밀번호를 사용합니다.

### 4. 세션 복원

기기 B에 프로젝트 작업 사본을 준비하고 연결합니다.

~~~bash
cd /path/to/project
ctxhop project bind --path .
ctxhop list
ctxhop resume <SESSION_ID>
~~~

복원한 뒤에는 에이전트의 네이티브 명령으로 이어서 작업합니다.

~~~bash
# Codex
codex resume <SESSION_ID>

# Claude Code
claude --resume <SESSION_ID>
~~~

### 선택: 다른 에이전트로 전환

에이전트 간 전환은 대상 에이전트의 네이티브 세션을 새로 만들고, 원본 세션은 바꾸지 않습니다.

~~~bash
# 전환 미리 보기
ctxhop session switch <SESSION_ID> --to codex --preview

# 대상 세션을 만들고 실행
ctxhop session switch <SESSION_ID> --to codex --launch
~~~

Claude Code로 전환하려면 `--to claude-code`를 사용합니다.

## 동기화하는 데이터

CtxHop은 기본적으로 암호화된 세션 컨텍스트와 프로젝트 식별 정보를 동기화합니다. 작업 공간 파일과 Git 상태는 `--workspace`를 쓸 때만 포함합니다. 에이전트 설정은 동기화하기 전에 걸러 냅니다.

| 데이터 | 범위 |
|---|---|
| 세션 컨텍스트 | 기본으로 동기화합니다. 암호화된 에이전트 세션 기록으로 저장합니다. |
| 프로젝트 식별 정보와 Git 요약 | 기본으로 동기화합니다. 여러 기기에서 같은 프로젝트를 찾는 데 씁니다. |
| 에이전트 환경 | `init` 때 고른 구성 요소를 걸러서 포함합니다. 스킬, MCP 인텐트, 허용된 세션 설정 등이 여기에 속합니다. |
| 작업 공간과 Git 상태 | 선택 사항입니다. `push --workspace`와 `resume --workspace`를 쓸 때만 포함합니다. |
| 자격 증명과 비밀 정보 | 절대 동기화하지 않습니다. 토큰, 개인 키, 인증 파일, 헤더, 환경 변수의 비밀 값, `.env` 파일이 여기에 속합니다. |

프로젝트 파일과 Git 저장소 전체는 기본 동기화 범위에 들어가지 않습니다.

## CLI

옵션은 `ctxhop <command> --help`로 확인합니다. 명령 목록은 `ctxhop help <command> [action]`으로 살펴봅니다. 터미널에서 `ctxhop`을 실행하면 대화형 작업 공간이 열리고, 입출력을 리디렉션하면 명령 목록을 출력합니다. 글자를 입력해 목록을 거르고, 화살표 키로 이동하고, Enter를 눌러 동작을 실행합니다. `[--json]`이 붙은 명령은 기계가 읽을 수 있는 출력을 지원합니다.

`<HUB>`, `<PROJECT_ID>`, `<SESSION_ID>`, `<REPLICA_ID>`, `<CONTRIBUTION_ID>`, `<NATIVE_ID>`는 사용자가 직접 넣는 선택자입니다.

### 설정과 이동

| 명령 | 설명 |
|---|---|
| `ctxhop` | 대화형 작업 공간을 엽니다. |
| `ctxhop init [options]` | 스토리지, 암호화, 기기 식별 정보, 에이전트 훅을 설정합니다. |
| `ctxhop install [--dir DIR] [--no-path]` | CtxHop 명령을 설치합니다. |
| `ctxhop update` | 최신 릴리스를 확인하고 설치합니다. |
| `ctxhop uninstall [--dir DIR]` | 로컬 CtxHop 설치를 제거합니다. |
| `ctxhop help [<command> [action]]` | 명령 목록이나 명령 옵션을 보여 줍니다. |
| `ctxhop version` | 설치된 버전을 보여 줍니다. |

### 프로젝트와 동기화

| 명령 | 설명 |
|---|---|
| `ctxhop project bind [--path DIR] [--identity ID or --name NAME] [--hub HUB]` | 프로젝트를 고정 식별자와 허브에 연결합니다. |
| `ctxhop project unbind [--path DIR or --identity ID]` | 프로젝트 연결을 해제합니다. |
| `ctxhop project mode <MODE> [--path DIR or --identity ID]` | 프로젝트 동기화 모드를 `normal`, `push-only`, `excluded` 중 하나로 설정합니다. |
| `ctxhop project list [--hub HUB] [--json]` | 프로젝트 연결 목록을 보여 줍니다. |
| `ctxhop project discover [--json]` | 승인된 기기에서 프로젝트를 찾습니다. |
| `ctxhop project move <PROJECT_ID> --to <HUB> [--json]` | 프로젝트를 다른 허브로 옮깁니다. |
| `ctxhop push [--workspace] [--git-stash STASH] [SESSION_ID]` | 프로젝트 세션과 고른 환경을 푸시합니다. `--workspace`를 쓰면 작업 공간과 Git 상태도 포함합니다. |
| `ctxhop pull [--json]` | 원격 메타데이터를 읽습니다. |
| `ctxhop list [--json]` | 현재 프로젝트의 세션 목록을 보여 줍니다. |
| `ctxhop resume [SESSION_ID] [options]` | 세션과 고른 환경을 복원합니다. |
| `ctxhop watch [--interval DURATION] [--once] [--json]` | 로컬 에이전트 세션을 지켜보다가 변경 사항을 푸시합니다. |

선택자를 주지 않으면 `ctxhop list`와 `ctxhop resume`은 세션 선택 화면을 엽니다. 논리 세션, 에이전트 원본, 복제본을 보려면 `ctxhop session list`를 사용합니다.

### 허브와 논리 세션

Session Hub는 각 에이전트의 네이티브 세션을 논리 세션으로 정리하고 원본 관계를 유지합니다. 같은 에이전트에서 이어 가려면 `session resume`을, 다른 에이전트에서 이어 가려면 `switch`를 사용합니다.

| 명령 | 설명 |
|---|---|
| `ctxhop hub create [--json] <HUB>` | 허브를 만들고 게시합니다. |
| `ctxhop hub list [--json]` | 허브 목록과 현재 허브를 보여 줍니다. |
| `ctxhop hub use [--json] <HUB>` | 현재 허브를 선택합니다. |
| `ctxhop session discover [--json]` | 로컬 에이전트 세션과 그 허브 연결을 찾습니다. |
| `ctxhop session list [--json]` | 논리 세션, 에이전트 원본, 복제본 목록을 보여 줍니다. |
| `ctxhop session show <SESSION_ID> [--json]` | 논리 세션과 그 원본 메타데이터를 보여 줍니다. |
| `ctxhop session resume <SESSION_ID> [options]` | 현재 기기에서 네이티브 복제본을 이어 갑니다. |
| `ctxhop session switch <SESSION_ID> [options]` | 고른 컨텍스트로 대상 에이전트의 네이티브 세션을 미리 보거나 만듭니다. |
| `ctxhop session attach <SESSION_ID> [options] [--json]` | 기존 네이티브 세션을 논리 세션에 붙입니다. |
| `ctxhop session reconcile [options] [--json]` | 네이티브 세션 상태를 허브 연결과 비교합니다. |
| `ctxhop session migrate [--json] [--preview] [--publish-v2] [--rollback] [SESSION_ID]` | 이전 형식의 세션 메타데이터를 논리 세션 보기로 옮깁니다. |

전환 옵션:

| 옵션 | 설명 |
|---|---|
| `--to AGENT` | 대상 에이전트를 고릅니다. |
| `--context causal-head`, `all-heads` 또는 `agent-only` | 컨텍스트 정책을 고릅니다. |
| `--head CONTRIBUTION_ID` | 인과 헤드를 고릅니다. 여러 헤드를 고르려면 반복해서 씁니다. |
| `--source AGENT` | `agent-only`에 쓸 원본 에이전트를 고릅니다. |
| `--preview` | 로컬 상태를 바꾸지 않고 변환 계획을 미리 봅니다. 생략하면 바로 전환합니다. |
| `--with-environment` | 이식 가능한 환경 구성 요소를 전환에 포함합니다. |
| `--launch` | 전환한 뒤 대상 에이전트를 실행합니다. |
| `--allow-unsupported` | 지원하지 않는 기록을 미리 보기 보고서에 포함합니다. |

마이그레이션 명령:

~~~bash
ctxhop session migrate --preview
ctxhop session migrate <SESSION_ID> --publish-v2
ctxhop session migrate <SESSION_ID> --rollback
~~~

`--preview`가 없으면 고른 마이그레이션을 바로 실행합니다. `--publish-v2`는 고른 이전 형식 브랜치를 복제본으로 게시하고, `--rollback`은 이전 형식 리더를 선택합니다.

### 기기와 보안

| 명령 | 설명 |
|---|---|
| `ctxhop device invite [--output PATH]` | 기기 초대 파일을 만듭니다. |
| `ctxhop device status [--json]` | 로컬 기기 식별 정보와 모드를 보여 줍니다. |
| `ctxhop device mode <MODE>` | 기기 모드를 설정합니다. |
| `ctxhop device list [--json]` | 승인된 기기 목록을 보여 줍니다. |
| `ctxhop device rename <NAME>` | 로컬 기기 이름을 바꿉니다. |
| `ctxhop device remove <DEVICE_ID>` | 기기 승인을 취소합니다. |
| `ctxhop device rotate-key` | 암호화 키를 교체합니다. |
| `ctxhop passphrase change` | 암호화 비밀번호를 바꿉니다. |
| `ctxhop passphrase reset` | 복구 키로 암호화 비밀번호를 재설정합니다. |
| `ctxhop hook install [--agent all, claude-code, or codex]` | 고른 에이전트에 SessionEnd 훅을 설치합니다. |

### 기록과 원격 데이터

| 명령 | 설명 |
|---|---|
| `ctxhop history [--json] <SESSION_ID>` | 세션 버전 목록을 보여 줍니다. |
| `ctxhop history cleanup [--remote-id] [--path DIR] <SESSION_ID>` | 세션의 모든 원격 버전을 삭제합니다. |
| `ctxhop history prune [--remote-id] [--path DIR] (--keep N or --before RFC3339) <SESSION_ID>` | 최신 버전만 남기거나 지정한 시각 이전의 버전을 삭제합니다. |
| `ctxhop remote delete-session [--remote-id] [--path DIR] <SESSION_ID>` | 원격 세션을 삭제합니다. |
| `ctxhop remote delete-project [--path DIR]` | 현재 프로젝트의 원격 데이터를 삭제합니다. |
| `ctxhop remote delete-all` | 현재 동기화 도메인의 원격 데이터를 삭제합니다. |

불투명한 원격 ID를 대상으로 하려면 `--remote-id`를 사용합니다.

### 상태와 유지 관리

| 명령 | 설명 |
|---|---|
| `ctxhop status [--remote] [--json]` | 동기화 상태를 보여 줍니다. `--remote`를 쓰면 원격 상태도 포함합니다. |
| `ctxhop doctor [--json]` | 설정, 백엔드, 에이전트, 프로젝트, 훅 상태를 점검합니다. |
| `ctxhop stats [--json]` | 기기 간 복원 통계를 보여 줍니다. |

## 설정

CtxHop은 로컬 설정, 기기 키, 동기화 상태를 다음 위치에 저장합니다.

| 시스템 | 기본 디렉터리 |
|---|---|
| Windows | `%USERPROFILE%\.ctxhop` |
| macOS / Linux | `~/.ctxhop` |

다른 디렉터리를 쓰려면 `CTXHOP_CONFIG_DIR`을 설정합니다.

~~~bash
export CTXHOP_CONFIG_DIR="$HOME/.ctxhop-custom"
~~~

PowerShell:

~~~powershell
$env:CTXHOP_CONFIG_DIR = Join-Path $env:USERPROFILE '.ctxhop-custom'
~~~

이 디렉터리에는 로컬 설정과 기기 키가 들어 있습니다. 저장소에 커밋하거나 공개적으로 공유하지 마세요.

## Windows GUI (이 포크)

이 포크는 [`gui/ctxhop-gui-vnext/`](gui/ctxhop-gui-vnext/)에 Windows GUI를 추가했습니다. Windows PowerShell 5.1과 WinForms로 만든 화면이며, `Run-CtxHop-GUI-vNext.cmd`로 실행합니다.

- Claude Code와 Codex Desktop 대화를 한 창에서 백업하고 복원합니다. 백업은 한 번에 대화 하나씩 합니다. Codex Desktop 복원은 고른 백업을 먼저 미리 보여 주고, 항목마다 처리 방법을 직접 고르게 합니다(모든 항목의 기본값은 건너뛰기).
- 화면은 한국어나 영어로 볼 수 있습니다. 설정 탭의 **Language / 언어**에서 고른 뒤 프로그램을 다시 시작하면 적용됩니다.
- Codex Desktop 대화 전송에는 새로 추가한 `ctxhop bundle` 명령을 씁니다.
- GUI를 쓰려면 빌드한 `bin\ctxhop.exe`와 `bin\ctxhop-claude.exe`가 있어야 합니다. GUI는 두 파일을 SHA-256으로 고정해 확인하며, 두 파일은 저장소에 커밋되어 있지 않습니다.
- 아직 검토 후보입니다. 두 PC 사이의 실제 왕복은 아직 실행하지 않았습니다.

사용 순서와 복구 절차는 [GUI README](gui/ctxhop-gui-vnext/README.md)를 참고하세요.

## 개발

Go 1.26 이상이 필요합니다.

저장소를 복제하고 빌드합니다.

~~~bash
git clone https://github.com/CCCCY-ci/ctxhop.git
cd ctxhop
go build -trimpath -o ctxhop ./cmd/ctxhop
./ctxhop install
~~~

Windows PowerShell에서는 다음과 같이 합니다.

~~~powershell
go build -trimpath -o ctxhop.exe ./cmd/ctxhop
.\ctxhop.exe install
~~~

기본 검사를 실행합니다.

~~~bash
go test ./...
go vet ./...
~~~

변경 사항을 제출하기 전에 다음을 실행합니다.

~~~bash
go test -race ./...
~~~

지원하는 모든 대상을 빌드합니다.

~~~bash
bash scripts/build.sh
~~~

Windows PowerShell에서는 다음과 같이 합니다.

~~~powershell
.\scripts\build.ps1
~~~

실제 세션 파일, 토큰, 백엔드 자격 증명을 커밋하지 마세요.

## 라이선스

CtxHop은 [MIT 라이선스](LICENSE)로 배포됩니다.
