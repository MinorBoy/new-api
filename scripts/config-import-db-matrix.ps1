$ErrorActionPreference = 'Stop'

$composeFile = Join-Path $PSScriptRoot '..\docker-compose.config-import-test.yml'
$originalSqlDsn = $env:SQL_DSN
$originalMySqlDsn = $env:TEST_MYSQL_DSN
$originalPostgresDsn = $env:TEST_POSTGRES_DSN

function Restore-EnvironmentValue([string]$name, [string]$value) {
  if ($null -eq $value) {
    Remove-Item "Env:$name" -ErrorAction SilentlyContinue
    return
  }
  Set-Item "Env:$name" $value
}

try {
  docker compose -f $composeFile up -d | Out-Host

  $deadline = (Get-Date).AddMinutes(3)
  do {
    $savedErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    $mysqlReady = docker compose -f $composeFile exec -T mysql mysqladmin ping -uroot 2>$null
    $mysqlExitCode = $LASTEXITCODE
    $postgresReady = docker compose -f $composeFile exec -T postgres pg_isready -U postgres -d postgres 2>$null
    $postgresExitCode = $LASTEXITCODE
    $ErrorActionPreference = $savedErrorActionPreference
    if ($mysqlExitCode -eq 0 -and $postgresExitCode -eq 0 -and $mysqlReady -and $postgresReady) { break }
    Start-Sleep -Seconds 2
  } while ((Get-Date) -lt $deadline)

  if ($mysqlExitCode -ne 0 -or $postgresExitCode -ne 0 -or -not $mysqlReady -or -not $postgresReady) {
    throw 'Timed out waiting for the config-import database matrix.'
  }

  docker compose -f $composeFile exec -T mysql mysql -uroot -e "CREATE USER IF NOT EXISTS 'config_import'@'%' IDENTIFIED BY 'config_import_test'; GRANT ALL PRIVILEGES ON newapi.* TO 'config_import'@'%'; FLUSH PRIVILEGES;" | Out-Host

  $env:TEST_MYSQL_DSN = 'config_import:config_import_test@tcp(127.0.0.1:13306)/newapi?charset=utf8mb4&loc=Local&parseTime=true'
  $env:TEST_POSTGRES_DSN = 'postgres://postgres:config_import_test@127.0.0.1:15432/newapi?sslmode=disable'

  Remove-Item Env:SQL_DSN -ErrorAction SilentlyContinue
  go test ./model -run 'TestConfigImportMigrationConfiguredDatabases|TestCostAccountingMigrationConfiguredDatabases' -count=1
  go test ./service -run 'TestConfigImport' -count=1
  go test ./e2e -run 'TestConfigImport' -count=1
}
finally {
  Restore-EnvironmentValue 'SQL_DSN' $originalSqlDsn
  Restore-EnvironmentValue 'TEST_MYSQL_DSN' $originalMySqlDsn
  Restore-EnvironmentValue 'TEST_POSTGRES_DSN' $originalPostgresDsn
  docker compose -f $composeFile down | Out-Host
}
