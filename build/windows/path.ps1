# Permanent native installer helper: add or remove only SafeRM's user PATH directory.
param(
    [Parameter(Mandatory = $true)][ValidateSet('Add', 'Remove')][string]$Action,
    [Parameter(Mandatory = $true)][string]$InstallDirectory
)
$ErrorActionPreference = 'Stop'
$environmentKey = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Environment')
try {
    # Retain unexpanded variables and avoid NSIS's fixed string length limit.
    $userPath = [string]$environmentKey.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
    $pathDirectories = @($userPath.Split(';') | Where-Object { $_ -and $_.TrimEnd('\') -ine $InstallDirectory.TrimEnd('\') })
    if ($Action -eq 'Add') { $pathDirectories += $InstallDirectory }
    $environmentKey.SetValue('Path', ($pathDirectories -join ';'), [Microsoft.Win32.RegistryValueKind]::ExpandString)
} finally {
    $environmentKey.Dispose()
}
