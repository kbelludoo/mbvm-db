package impact

import (
 "fmt"
 "os"
 "path/filepath"
 "strings"
)

// CompareRealModuleGitRefs reads TWO actual, already committed Git
// revisions and checks old client source against the real new provider.
// No checkout, worktree mutation, commit rewriting or source writes.
func CompareRealModuleGitRefs(repo,oldRef,newRef,symbol string)(RealModuleDiff,error){
 var empty RealModuleDiff
 if oldRef==""||newRef==""||strings.HasPrefix(oldRef,"-")||strings.HasPrefix(newRef,"-"){
  return empty,fmt.Errorf("both local Git refs must be valid and non-empty")
 }
 root,err:=filepath.Abs(repo)
 if err!=nil{return empty,err}
 temp,err:=os.MkdirTemp("","mbvm-real-git-diff-")
 if err!=nil{return empty,err}
 defer os.RemoveAll(temp)
 before:=filepath.Join(temp,"old")
 after:=filepath.Join(temp,"new")
 if err:=extractGitTree(root,oldRef,before);err!=nil{
  return empty,fmt.Errorf("archive old Git ref %q: %w",oldRef,err)
 }
 if err:=extractGitTree(root,newRef,after);err!=nil{
  return empty,fmt.Errorf("archive new Git ref %q: %w",newRef,err)
 }
 return CompareRealModuleDirs(before,after,symbol)
}
