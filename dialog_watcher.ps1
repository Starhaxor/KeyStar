Add-Type -AssemblyName UIAutomationClient
$deadline = (Get-Date).AddSeconds(90)
$out = "c:\Users\pc\Desktop\Projelerim\KeyStar\dialog_capture.txt"
if (Test-Path $out) { Remove-Item $out }
while ((Get-Date) -lt $deadline) {
  $root = [System.Windows.Automation.AutomationElement]::RootElement
  $cond = New-Object System.Windows.Automation.PropertyCondition([System.Windows.Automation.AutomationElement]::NameProperty, "Sign-in failed")
  $el = $root.FindFirst([System.Windows.Automation.TreeScope]::Children, $cond)
  if ($el -ne $null) {
    $lines = @()
    $all = $el.FindAll([System.Windows.Automation.TreeScope]::Descendants, [System.Windows.Automation.Condition]::TrueCondition)
    foreach ($n in $all) { $lines += ($n.Current.ControlType.ProgrammaticName + " | " + $n.Current.Name) }
    ($lines -join "`r`n") | Out-File -FilePath $out -Encoding utf8
    exit 0
  }
  Start-Sleep -Milliseconds 150
}
"NOT_FOUND" | Out-File -FilePath $out -Encoding utf8
exit 1
