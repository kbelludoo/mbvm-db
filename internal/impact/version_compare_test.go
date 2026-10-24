package impact

import (
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func diffFixture(t *testing.T,oldSource,newSource string)(string,string){
 t.Helper()
 root:=t.TempDir()
 oldDir:=filepath.Join(root,"old")
 newDir:=filepath.Join(root,"new")
 if err:=os.MkdirAll(oldDir,0755);err!=nil{t.Fatal(err)}
 if err:=os.MkdirAll(newDir,0755);err!=nil{t.Fatal(err)}
 for _,pair:=range [][2]string{{filepath.Join(oldDir,"pkg.go"),oldSource},{filepath.Join(newDir,"pkg.go"),newSource}}{
  if err:=os.WriteFile(pair[0],[]byte(pair[1]),0644);err!=nil{t.Fatal(err)}
 }
 return oldDir,newDir
}

func changeByName(t *testing.T,r VersionReport,name string)VersionChange{
 t.Helper()
 for _,c:=range r.Changes{
  if strings.HasSuffix(c.Symbol,"."+name){return c}
 }
 t.Fatalf("missing version change %s in %+v",name,r.Changes)
 return VersionChange{}
}

func TestCompareDirsSignatureBreakageFromActualVersion(t *testing.T){
 oldDir,newDir:=diffFixture(t,`package fixture
func Sum(x int) int {return x}
func Caller() int {return Sum(2)}
func Transitive() int{return Caller()}
func Retired() int {return 7}
`, `package fixture
func Sum(x int,y int) int{return x+y}
func Caller() int {return Sum(2,3)}
func Transitive() int{return Caller()}
func NewFunction() int{return 11}
`)
 report,err:=CompareDirs(oldDir,newDir)
 if err!=nil{t.Fatal(err)}
 if !report.SourceFilesUnmodified||report.OldPackage!=report.NewPackage{t.Fatalf("invalid version report %+v",report)}
 changed:=changeByName(t,report,"Sum")
 if changed.Kind!=SignatureChanged||changed.Compatibility==nil||
  !changed.Compatibility.Supported||changed.Compatibility.HybridPackageTypechecked{
  t.Fatalf("missing real old/new signature compile witness: %+v",changed)
 }
 if len(changed.OldDirectCallers)!=1||len(changed.OldTransitiveCandidates)!=1||
  len(changed.Compatibility.BrokenOldDirectCallers)!=1||
  !strings.HasSuffix(changed.Compatibility.BrokenOldDirectCallers[0],".Caller"){
  t.Fatalf("incorrect old version impact %+v",changed)
 }
 if got:=changeByName(t,report,"Caller");got.Kind!=BodyChanged{
  t.Fatalf("caller changed body must remain unproven: %+v",got)
 }
 if got:=changeByName(t,report,"Retired");got.Kind!=Removed{
  t.Fatalf("removal not tracked: %+v",got)
 }
 if got:=changeByName(t,report,"NewFunction");got.Kind!=Added{
  t.Fatalf("addition not tracked: %+v",got)
 }
 t.Logf("Old callers against new function contract: %+v",changed.Compatibility)
}

func TestCompareDirsMethodIdentityNoHomonymCollision(t *testing.T){
 oldDir,newDir:=diffFixture(t,`package fixture
type A struct{}
type B struct{}
func (A) Apply(x int) int{return x}
func (B) Apply(x int) int{return x+1}
func CallA(a A) int{return a.Apply(2)}
func CallB(b B) int{return b.Apply(2)}
`, `package fixture
type A struct{}
type B struct{}
func (A) Apply(x,y int) int{return x+y}
func (B) Apply(x int) int{return x+1}
func CallA(a A) int{return a.Apply(2,3)}
func CallB(b B) int{return b.Apply(2)}
`)
 report,err:=CompareDirs(oldDir,newDir)
 if err!=nil{t.Fatal(err)}
 var method *VersionChange
 for i:=range report.Changes {
  c:=&report.Changes[i]
  if strings.HasSuffix(c.Symbol,"(A).Apply"){method=c}
  if strings.HasSuffix(c.Symbol,"(B).Apply"){t.Fatalf("unchanged B.Apply unexpectedly flagged: %+v",c)}
 }
 if method==nil||method.Kind!=SignatureChanged||method.Compatibility==nil||
  !method.Compatibility.Supported||len(method.Compatibility.BrokenOldDirectCallers)!=1||
  !strings.HasSuffix(method.Compatibility.BrokenOldDirectCallers[0],".CallA"){
  t.Fatalf("typed receiver identity not preserved: %+v",report.Changes)
 }
}

func TestCompareDirsRefactorUnprovenAndNoFormattingFalsePositive(t *testing.T){
 oldDir,newDir:=diffFixture(t,`package fixture
func Rename(x int) int { return x + 1 }
func Unchanged(y int) int{return y*2}
`, `package fixture
func Rename(z int) int { return z + 1 }
func Unchanged(y int) int { return y * 2 }
`)
 report,err:=CompareDirs(oldDir,newDir)
 if err!=nil{t.Fatal(err)}
 if len(report.Changes)!=1||report.Changes[0].Kind!=BodyChanged||
  !strings.HasSuffix(report.Changes[0].Symbol,".Rename")||
  report.Changes[0].Compatibility!=nil {
  t.Fatalf("parameter alpha-renaming must not be called a signature break: %+v",report.Changes)
 }
}

func TestCompareDirsRejectsDifferentPackagePaths(t *testing.T){
 oldDir,newDir:=diffFixture(t,`package oldname
func F() {}
`, `package newname
func F() {}
`)
 if _,err:=CompareDirs(oldDir,newDir);err==nil{
  t.Fatal("different package identities must fail rather than conflate symbols")
 }
}
