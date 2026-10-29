param([string]$FreshCRTDirectory = '', [string]$OutputDirectory = $PSScriptRoot)

$ErrorActionPreference = 'Stop'
$auditRoot = (Resolve-Path (Join-Path $PSScriptRoot '../../..')).Path
New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
$auditOutput = (Resolve-Path -LiteralPath $OutputDirectory).Path
Push-Location $auditRoot
try {
    function Read-Report([string]$Path) {
        return Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
    }
    function Require([bool]$Condition, [string]$Message) {
        if (-not $Condition) { throw $Message }
    }
    function Save-JSON([string]$Name, $Value) {
        $Value | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath (Join-Path $auditOutput $Name) -Encoding utf8
    }

    $multi = Read-Report 'experiments/hdc-csr/multipath_quality.json'
    Require ($multi.seeds -eq 50 -and $multi.queries + $multi.skipped_no_relevant -eq 150) 'Unexpected multipath scope'
    $capacity = Read-Report 'experiments/hdc-csr/hdc_capacity_results.json'
    Require ($capacity.schema -eq 'mbvm-hdc-capacity-v1' -and $capacity.results.Count -eq 120) 'Unexpected capacity schema'
    $capacityRows = @($capacity.results | Group-Object length,bit_flip_rate | ForEach-Object {
        if ($_.Group[0].bit_flip_rate -eq 0.4 -and $_.Group[0].length -in @(3,8,32,64)) {
            [ordered]@{
                length = $_.Group[0].length
                noise = 0.4
                trials = ($_.Group.superposed_hdc.total_programs | Measure-Object -Sum).Sum
                hdc_exact_program_rate = ($_.Group.superposed_hdc.exact_program_rate | Measure-Object -Average).Average
                ecc_exact_program_rate = ($_.Group.equal_budget_repetition_code.exact_program_rate | Measure-Object -Average).Average
            }
        }
    })
    $pilot = Read-Report 'experiments/real_eval/pilot_results.json'
    $ablation = Read-Report 'experiments/real_eval/ablation_26881_results.json'
    function Summarize-Calls($Rows) {
        return @($Rows | Group-Object condition | ForEach-Object {
            [ordered]@{
                condition = $_.Name
                issued = $_.Count
                completed = @($_.Group | Where-Object { $_.model_version -ne '' }).Count
                marked_success = @($_.Group | Where-Object success).Count
            }
        })
    }
    $ls = Read-Report 'experiments/hdc-csr/lsh_quality.json'
    $historic = [ordered]@{
        status = 'saved_reports_only_not_rerun_with_race'
        multipath = $multi
        capacity = [ordered]@{
            configurations = $capacity.results.Count
            program_trials_per_approach = ($capacity.results.superposed_hdc.total_programs | Measure-Object -Sum).Sum
            numeric_evaluations_per_approach = ($capacity.results.superposed_hdc.total_answers | Measure-Object -Sum).Sum
            selected_40_percent_noise_results = $capacityRows
        }
        binary_lsh = $ls
        pilot = Summarize-Calls $pilot.results
        ablation_single_n = Summarize-Calls $ablation.results
        typed_package_breakage = Read-Report 'experiments/hdc-csr/typed_real_impact_output.json'
        caveat = 'Flags and summaries in historical JSON are not independent execution certificates.'
    }
    Save-JSON 'historical_summary.json' $historic

    # Validate exact numerical identities without the repository's Go verifier.
    $identities = @($ablation.results | Where-Object success | ForEach-Object {
        $match = [regex]::Match($_.verification_status, '4/(\d+) = 1/(\d+) \+ 1/(\d+) \+ 1/(\d+)')
        Require $match.Success 'Successful numeric result lacks an explicit identity'
        $n,$x,$y,$z = 1..4 | ForEach-Object { [System.Numerics.BigInteger]::Parse($match.Groups[$_].Value) }
        $valid = $n -gt 0 -and $x -gt 0 -and $y -gt 0 -and $z -gt 0 -and 4*$x*$y*$z -eq $n*($x*$y+$x*$z+$y*$z)
        Require $valid 'Invalid exact Erdos-Straus numeric identity'
        [ordered]@{ n="$n"; x="$x"; y="$y"; z="$z"; exact_integer_identity=$valid }
    })

    # Extract only the small public fixture tables, not source packages.
    $pairSource = Get-Content -LiteralPath 'experiments/real_eval/multilemma_dev_runner.go' -Raw
    $tripleSource = Get-Content -LiteralPath 'experiments/real_eval/multilemma_depth_dev.go' -Raw
    $pairBlock = [regex]::Match($pairSource, '(?s)fixtures := \[\]\[4\]int64\{(.*?)\r?\n\s*\}').Groups[1].Value
    $thirdBlock = [regex]::Match($tripleSource, '(?s)third := \[\]\[2\]int64\{(.*?)\r?\n\s*\}').Groups[1].Value
    $pairs = @([regex]::Matches($pairBlock, '\{(\d+), (\d+), (\d+), (\d+)\}'))
    $thirds = @([regex]::Matches($thirdBlock, '\{(\d+), (\d+)\}'))
    Require ($pairs.Count -eq 12 -and $thirds.Count -eq 12) 'Expected exactly twelve frozen CRT fixtures'
    $crtChecks = @()
    $crtInputs = @()
    foreach ($file in @('crt_dev_results.json','crt_depth_dev_results.json','crt_choice_dev_results.json')) {
        $path = if ($FreshCRTDirectory) { Join-Path $FreshCRTDirectory $file } else { Join-Path $auditRoot $file }
        $report = Read-Report $path
        Require ($report.results.Count -eq 12) 'Expected twelve CRT rows per report'
        $crtInputs += [ordered]@{ file=$file; sha256=(Get-FileHash -LiteralPath $path).Hash.ToLowerInvariant() }
        for ($i=0; $i -lt 12; $i++) {
            $row = $report.results[$i]
            $base = $pairs[$i]
            $mods = @([int]$base.Groups[1].Value,[int]$base.Groups[3].Value)
            $rems = @([int]$base.Groups[2].Value,[int]$base.Groups[4].Value)
            if ($file -ne 'crt_dev_results.json') {
                $mods += [int]$thirds[$i].Groups[1].Value
                $rems += [int]$thirds[$i].Groups[2].Value
            }
            $product=1
            foreach ($mod in $mods) { $product *= $mod }
            $certificates = @($row.conventional.certificate,$row.csr.certificate)
            foreach ($certificate in $certificates) {
                if ($file -ne 'crt_dev_results.json') {
                    $mid = $certificate.intermediate
                    Require ([int]$mid.modulus -eq $mods[0]*$mods[1]) 'Invalid intermediate CRT modulus'
                    Require ([int]$mid.residue -ge 0 -and [int]$mid.residue -lt [int]$mid.modulus) 'Noncanonical intermediate CRT residue'
                    Require ([int]$mid.residue % $mods[0] -eq $rems[0] -and [int]$mid.residue % $mods[1] -eq $rems[1]) 'Invalid intermediate CRT class'
                    Require ([int]$certificate.final.residue % [int]$mid.modulus -eq [int]$mid.residue) 'CRT stages disagree'
                    $certificate=$certificate.final
                }
                $residue=[int]$certificate.residue
                Require ([int]$certificate.modulus -eq $product -and $residue -ge 0 -and $residue -lt $product) 'Invalid CRT canonical class'
                for ($j=0; $j -lt $mods.Count; $j++) {
                    Require ($residue % $mods[$j] -eq $rems[$j]) 'CRT certificate violates a premise'
                }
            }
            # Independent exhaustive oracle for these small, finite moduli.
            $allMatches=0
            $without = New-Object 'int[]' $mods.Count
            for ($candidate=0; $candidate -lt $product; $candidate++) {
                $failures=@()
                for ($j=0; $j -lt $mods.Count; $j++) {
                    if ($candidate % $mods[$j] -ne $rems[$j]) { $failures += $j }
                }
                if ($failures.Count -eq 0) { $allMatches++ }
                for ($j=0; $j -lt $mods.Count; $j++) {
                    if ($failures.Count -eq 0 -or ($failures.Count -eq 1 -and $failures[0] -eq $j)) { $without[$j]++ }
                }
            }
            Require ($allMatches -eq 1 -and @($without | Where-Object { $_ -le 1 }).Count -eq 0) 'CRT uniqueness or premise indispensability failed'
            $left = $row.conventional.certificate | ConvertTo-Json -Depth 6 -Compress
            $right = $row.csr.certificate | ConvertTo-Json -Depth 6 -Compress
            Require ($left -eq $right) 'Conventional and CSR certificates differ'
            $crtChecks += [ordered]@{ report=$file; task=$row.task_id; residue=$residue; modulus=$product; canonical_unique_class=$true; every_premise_indispensable_for_this_class=$true; matching_certificates=$true }
        }
    }
    Save-JSON 'arithmetic_checks.json' ([ordered]@{
        method='Independent .NET BigInteger identities and exhaustive enumeration of public CRT fixture residue classes'
        crt_inputs=if ($FreshCRTDirectory) {'fresh_go_run_outputs'} else {'historical_reports'}
        numeric_identities_checked=$identities.Count
        identities=$identities
        crt_report_hashes=$crtInputs
        crt_tasks_checked=$crtChecks.Count
        crt_arm_certificates_checked=2*$crtChecks.Count
        crt_checks=$crtChecks
        scope='Finite certificates only. This does not prove new theorems or validate prose proofs, model provenance, retrieval traces, or generalization.'
    })

    $artifacts = @('codebase.mbvmdb','README.md','experiments/hdc-csr/quality_output.json','experiments/hdc-csr/multipath_quality.json','experiments/hdc-csr/lsh_quality.json','experiments/hdc-csr/lsh_sweep_quality.json','experiments/hdc-csr/hdc_capacity_results.json','experiments/hdc-csr/composition_lab_results.json','experiments/hdc-csr/dynamic_composition_results.json','experiments/hdc-csr/typed_real_impact_output.json','experiments/hdc-csr/real_world_impact_output.json','experiments/hdc-csr/web/official_benchmarks.json','experiments/real_eval/pilot_results.json','experiments/real_eval/ablation_26881_results.json','experiments/erdos_straus/erdos_straus_results.json','crt_dev_results.json','crt_depth_dev_results.json','crt_choice_dev_results.json','kaggle_output.zip','mbvm_neural_weights.bin')
    $artifacts | ForEach-Object { (Get-FileHash -LiteralPath $_).Hash.ToLowerInvariant() + '  ' + $_ } | Set-Content -LiteralPath (Join-Path $auditOutput 'artifacts.sha256') -Encoding utf8
    $sources = @(& rg --files --hidden -g '!.git/**' -g '*.go' -g '*.py' -g '*.yml' -g 'go.mod' -g 'go.sum' -g 'AGENTS.md' -g 'AGENTS.mbvm' | Sort-Object)
    Require ($LASTEXITCODE -eq 0) 'Source inventory failed'
    $sources | ForEach-Object { (Get-FileHash -LiteralPath $_).Hash.ToLowerInvariant() + '  ' + $_.Replace('\','/') } | Set-Content -LiteralPath (Join-Path $auditOutput 'source.sha256') -Encoding utf8
    [ordered]@{ crt_tasks_checked=$crtChecks.Count; numeric_identities_checked=$identities.Count; source_files_hashed=$sources.Count; artifact_files_hashed=$artifacts.Count } | ConvertTo-Json
} finally {
    Pop-Location
}
