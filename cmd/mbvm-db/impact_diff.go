package main

import (
 "encoding/json"
 "flag"
 "fmt"
 "io"
 "strings"

 "github.com/kbelludoo/mbvm-db/internal/impact"
)

// runImpactDiff compares two existing package directories, including
// directories inside separate Git checkouts. It does not use worktrees,
// git checkout, or any source-file writes.
func runImpactDiff(args []string,out io.Writer)error{
 flags:=flag.NewFlagSet("impact-diff",flag.ContinueOnError)
 flags.SetOutput(io.Discard)
 oldDir:=flags.String("old","","old Go package directory")
 newDir:=flags.String("new","","new Go package directory")
 gitRepo:=flags.String("repo",".","local Git repository root (revision mode)")
 oldRef:=flags.String("old-ref","","old local Git branch/tag/commit (revision mode)")
 newRef:=flags.String("new-ref","","new local Git branch/tag/commit (revision mode)")
 packageRel:=flags.String("package",".","repository-relative Go package path (revision mode)")
 jsonOutput:=flags.Bool("json",false,"emit machine-readable JSON")
 if err:=flags.Parse(args);err!=nil{return fmt.Errorf("impact-diff flags: %w",err)}
 if len(flags.Args())!=0{return fmt.Errorf("unexpected arguments: %s",strings.Join(flags.Args()," "))}
 var report impact.VersionReport
 var err error
 revisionMode:=*oldRef!=""||*newRef!=""
 if revisionMode{
  if *oldRef==""||*newRef==""{
   return fmt.Errorf("both -old-ref and -new-ref must be supplied")
  }
  if *oldDir!=""||*newDir!=""{
   return fmt.Errorf("choose either -old/-new directories or -old-ref/-new-ref revisions, not both")
  }
  report,err=impact.CompareGitRefs(*gitRepo,*oldRef,*newRef,*packageRel)
 }else{
  if strings.TrimSpace(*oldDir)==""||strings.TrimSpace(*newDir)==""{
   return fmt.Errorf("-old and -new package directories are required")
  }
  if *packageRel!="."||*gitRepo!="."{
   return fmt.Errorf("-repo and -package are only valid with -old-ref/-new-ref")
  }
  report,err=impact.CompareDirs(*oldDir,*newDir)
 }
 if err!=nil{return err}
 if *jsonOutput{
  encoder:=json.NewEncoder(out)
  encoder.SetIndent("","  ")
  return encoder.Encode(report)
 }
 fmt.Fprintf(out,"Package: %s\nOld source SHA-256: %s\nNew source SHA-256: %s\nChanges: %d\n",
  report.OldPackage,report.OldSourceSHA256,report.NewSourceSHA256,len(report.Changes))
 for _,change:=range report.Changes {
  fmt.Fprintf(out,"- %s: %s (old direct=%d, old transitive=%d)\n",
   change.Symbol,change.Kind,len(change.OldDirectCallers),len(change.OldTransitiveCandidates))
  if check:=change.Compatibility;check!=nil{
   if !check.Supported{
    fmt.Fprintf(out,"  Compatibility unknown: %s\n",check.Reason)
   }else{
    fmt.Fprintf(out,"  Old direct callers with type errors: %d\n",len(check.BrokenOldDirectCallers))
    for _,caller:=range check.BrokenOldDirectCallers{fmt.Fprintf(out,"    %s\n",caller)}
   }
  }
 }
 return nil
}
