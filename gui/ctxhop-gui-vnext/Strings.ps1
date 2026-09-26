#requires -Version 5.1
# 화면·작업 메시지 문자열. 각 항목은 키=@('한국어','English')이고, T 'Key' 인수…가 현재 언어 문장을 돌려준다.
# 인수는 -f 형식({0}, {1} …)으로 넣는다. 두 언어의 자리표시자는 같아야 한다(Test-Strings.ps1이 확인).
$script:UiLanguage='ko'
function Set-Language([string]$Language) { $script:UiLanguage=if ($Language -eq 'en') {'en'} else {'ko'} }
function T([string]$Key) {
    $pair=$script:StringTable[$Key]
    if ($null -eq $pair) { throw "Missing UI string: $Key" }
    $text=$pair[[int]($script:UiLanguage -eq 'en')]
    if ($args.Count) { return ($text -f $args) }
    return $text
}
$script:StringTable=@{
    # GUI.ps1

    # Worker.ps1

    # ClaudeWorker.ps1
}
