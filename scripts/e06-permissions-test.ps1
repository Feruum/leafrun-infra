$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'e06-common.ps1')
$e06PermissionTest=Join-Path $e06InfraRoot '.cache/e06/permissions-test.private'
[IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($e06PermissionTest))|Out-Null
[IO.File]::WriteAllText($e06PermissionTest,'non-secret permission test')
try {
 Set-E06PrivateFile $e06PermissionTest
 Set-E06PrivateFile $e06PermissionTest
 if($IsWindows){$current=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value;$allowed=(Get-Acl -LiteralPath $e06PermissionTest).Access|Where-Object{$_.AccessControlType -eq 'Allow'}|ForEach-Object{$_.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value};if($current -notin $allowed -or @($allowed|Where-Object{$_ -notin @($current,'S-1-5-18')}).Count){throw 'Private runtime file permits an unexpected identity'}}
} finally {Remove-Item -LiteralPath $e06PermissionTest -Force}
Write-Output 'Private file permissions allow only the normal user and SYSTEM'
