param(
    [string[]]$Reports = @(
        'output/gopls-comparison-2026-10-26/report.json',
        'output/gopls-comparison-2026-10-26-repeat-2/report.json',
        'output/gopls-comparison-2026-10-26-repeat-3/report.json',
        'output/gopls-comparison-clean-2026-10-26-1/report.json',
        'output/gopls-comparison-clean-2026-10-26-2/report.json',
        'output/gopls-comparison-clean-2026-10-26-3/report.json'
    ),
    [string]$Out = 'experiments/competitor_validation/validation.json'
)
$ErrorActionPreference = 'Stop'

function Assert-Equal($Actual, $Expected, [string]$Label) {
    if ($Actual -ne $Expected) { throw "$Label actual=$Actual expected=$Expected" }
}
function Canonical($Items) { return (@($Items | Sort-Object) -join [char]0) }
function Assert-Distribution($Items, $Reported, [string]$Label) {
    $ordered = @($Items | Sort-Object)
    Assert-Equal $Reported.samples $ordered.Count "$Label samples"
    $middle = [int][math]::Floor($ordered.Count / 2)
    $median = $ordered[$middle]
    if ($ordered.Count % 2 -eq 0) { $median = ($median + $ordered[$middle - 1]) / 2 }
    $p95 = $ordered[[int][math]::Ceiling(0.95 * $ordered.Count) - 1]
    if ([math]::Abs($Reported.median - $median) -gt 0.00000001) { throw "$Label median mismatch" }
    if ([math]::Abs($Reported.p95 - $p95) -gt 0.00000001) { throw "$Label p95 mismatch" }
}

$rows = @()
$baseline = $null
$pairedTotal = 0
$fixtureTotal = 0
$savedEditTotal = 0
$prefix = 'example.com/callfixture.'
$literalTarget = @(
    "$prefix(*Runner).Method", "${prefix}Async", "${prefix}Closure",
    "${prefix}Direct", "${prefix}Switch", 'example.com/callfixture/child.Cross'
)

foreach ($path in $Reports) {
    $report = Get-Content -LiteralPath $path -Raw | ConvertFrom-Json
    Assert-Equal $report.cases.Count 4 "$path case count"
    if ($null -eq $baseline) { $baseline = $report }
    Assert-Equal $report.gopls_binary_sha256 $baseline.gopls_binary_sha256 'gopls binary'
    Assert-Equal $report.mbvm_binary_sha256 $baseline.mbvm_binary_sha256 'MBVM binary'
    Assert-Equal $report.protocol_sha256 $baseline.protocol_sha256 'protocol'
    Assert-Equal $report.gomaxprocs 1 'GOMAXPROCS'
    Assert-Equal (@($report.cases.targets).Count) 38 'distinct query count'

    for ($caseIndex = 0; $caseIndex -lt 4; $caseIndex++) {
        $case = $report.cases[$caseIndex]
        Assert-Equal $case.error $null 'case error'
        Assert-Equal $case.sources_unchanged_before_fixture_edit $true 'source drift'
        Assert-Equal $case.source_before $case.source_after 'before/after source hashes'
        Assert-Equal $case.source_before $baseline.cases[$caseIndex].source_before 'same sources across launches and workspaces'
        Assert-Equal (Canonical $case.targets.symbol) (Canonical $baseline.cases[$caseIndex].targets.symbol) 'same selected targets'
        Assert-Equal $case.pairs.Count (7 * $case.targets.Count) 'equal fixed samples'
        foreach ($group in ($case.pairs | Group-Object symbol)) { Assert-Equal $group.Count 7 'samples per symbol' }
        Assert-Distribution $case.pairs.mbvm.milliseconds $case.mbvm_warm_ms 'MBVM'
        Assert-Distribution $case.pairs.gopls.milliseconds $case.gopls_warm_ms 'gopls'
        $exactA = 0
        $exactB = 0
        foreach ($pair in $case.pairs) {
            Assert-Equal $pair.mbvm.error $null 'MBVM query error'
            Assert-Equal $pair.gopls.error $null 'gopls query error'
            if ($caseIndex -eq 0) {
                $literal = @()
                if ($pair.symbol -eq "${prefix}Target") { $literal = $literalTarget }
                if ($pair.symbol -eq "${prefix}Generic") { $literal = @("${prefix}GenericCaller") }
                Assert-Equal (Canonical $pair.expected) (Canonical $literal) 'independent fixture labels'
                $fixtureTotal++
            }
            if ((Canonical $pair.mbvm.callers) -eq (Canonical $pair.expected)) { $exactA++ }
            if ((Canonical $pair.gopls.callers) -eq (Canonical $pair.expected)) { $exactB++ }
            $pairedTotal++
        }
        Assert-Equal $exactA $case.mbvm_exact_samples 'recomputed MBVM exact count'
        Assert-Equal $exactB $case.gopls_exact_samples 'recomputed gopls exact count'
        Assert-Equal $exactA $case.pairs.Count 'all MBVM sets'
        Assert-Equal $exactB $case.pairs.Count 'all gopls sets'
        foreach ($pair in $case.fixture_saved_edit) {
            $literal = @("${prefix}Switch")
            if ($pair.symbol -eq "${prefix}Target") { $literal = @($literalTarget | Where-Object { $_ -ne "${prefix}Switch" }) }
            Assert-Equal (Canonical $pair.expected) (Canonical $literal) 'saved-edit fixture label'
            Assert-Equal (Canonical $pair.mbvm.callers) (Canonical $literal) 'MBVM saved-edit answer'
            Assert-Equal (Canonical $pair.gopls.callers) (Canonical $literal) 'gopls saved-edit answer'
            $savedEditTotal++
        }
        $rows += [pscustomobject]@{
            Report = $path; Case = $caseIndex; Samples = $case.pairs.Count
            MBVMMedianMS = $case.mbvm_warm_ms.median; GoplsMedianMS = $case.gopls_warm_ms.median
            MBVMP95MS = $case.mbvm_warm_ms.p95; GoplsP95MS = $case.gopls_warm_ms.p95
            MBVMExact = $exactA; GoplsExact = $exactB; CacheHits = $case.mbvm_cache_hits
        }
    }
}

$result = [pscustomobject]@{
    Passed = $true; Reports = $Reports.Count; DistinctQueriesPerRun = 38
    PairedSamples = $pairedTotal; IndependentlyLabelledFixtureSamples = $fixtureTotal
    SavedEditPairs = $savedEditTotal; Rows = $rows
    Limits = 'Repeated queries are not independent datasets. Real-project labels are agreement with the typed MBVM graph. Memory and neural learning are not measured.'
}
$result | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $Out -Encoding utf8
Write-Output "PASS $($Reports.Count) reports; $pairedTotal paired queries; $fixtureTotal independently labelled fixture queries; $savedEditTotal saved-edit queries"
