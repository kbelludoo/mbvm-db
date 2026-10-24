package impact

import (
	"archive/tar"
	"bytes"
	"io"
	"strings"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitFixtureRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, string(output))
	}
}

func TestCompareGitRefsCommittedTreesOnly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git executable not available")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0755); err != nil {
		t.Fatal(err)
	}
	gitFixtureRun(t, root, "init")
	// Both revisions contain a TAR substantially larger than the default
	// Windows anonymous pipe buffer. Prior StdoutPipe+Wait code could hang
	// here even though the package being analyzed contains little Go.
	if err := os.MkdirAll(filepath.Join(root,"bulk"),0755);err!=nil{t.Fatal(err)}
	bulk:=[]byte(strings.Repeat("cross-platform-git-archive-stream\n",8192))
	if err:=os.WriteFile(filepath.Join(root,"bulk","stream.txt"),bulk,0600);err!=nil{t.Fatal(err)}

	filename := filepath.Join(root, "pkg", "example.go")
	write := func(src string) {
		t.Helper()
		if err := os.WriteFile(filename, []byte(src), 0600); err != nil {
			t.Fatal(err)
		}
	}
	commit := func(msg string) {
		t.Helper()
		gitFixtureRun(t, root, "add", "pkg/example.go", "bulk/stream.txt")
		gitFixtureRun(t, root, "-c", "user.name=MBVM Test", "-c", "user.email=mbvm@example.invalid", "commit", "-m", msg)
	}
	write("package example\nfunc F(a int) int {return a}\nfunc Call() int{return F(1)}\n")
	commit("v1")
	write("package example\nfunc F(a,b int) int {return a+b}\nfunc Call() int{return F(1,2)}\n")
	commit("v2")
	dirty := filepath.Join(root, "untracked.txt")
	if err := os.WriteFile(dirty, []byte("do not touch"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(dirty)
	if err != nil {
		t.Fatal(err)
	}
	report, err := CompareGitRefs(root, "HEAD~1", "HEAD", "pkg")
	if err != nil {
		t.Fatal(err)
	}
	changed := changeByName(t, report, "F")
	if changed.Kind != SignatureChanged || changed.Compatibility == nil ||
		!changed.Compatibility.Supported || len(changed.Compatibility.BrokenOldDirectCallers) != 1 {
		t.Fatalf("not a verified backward-compatibility break: %+v", changed)
	}
	after, err := os.ReadFile(dirty)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("dirty working tree was changed")
	}
	if _, err := CompareGitRefs(root, "--output=evil", "HEAD", "pkg"); err == nil {
		t.Fatal("accepted leading-dash Git ref")
	}
	if _, err := CompareGitRefs(root, "HEAD~1", "HEAD", "../escape"); err == nil {
		t.Fatal("accepted unsafe package path")
	}
	t.Logf("Git ref comparison found %d changed symbols without checkout or source-file writes", len(report.Changes))
}

func TestBoundedArchiveWriter(t *testing.T){
 var b bytes.Buffer
 writer:=&boundedArchiveWriter{dest:&b,remaining:5}
 if n,err:=writer.Write([]byte("hello"));err!=nil||n!=5{
  t.Fatalf("bounded write failed n=%d err=%v",n,err)
 }
 if n,err:=writer.Write([]byte("!"));err==nil||n!=0{
  t.Fatalf("writer must reject excess bytes without writing: n=%d err=%v",n,err)
 }
 if b.String()!="hello"{t.Fatalf("invalid bounded output: %s",b.String())}
}

func TestExtractGitArchivePAXMetadata(t *testing.T){
 if _,err:=exec.LookPath("git");err!=nil{t.Skip("Git executable not available")}
 repo:=t.TempDir()
 gitFixtureRun(t,repo,"init")
 source:=filepath.Join(repo,"example.go")
 if err:=os.WriteFile(source,[]byte("package example\nfunc Test(){}\n"),0600);err!=nil{t.Fatal(err)}
 gitFixtureRun(t,repo,"add","example.go")
 gitFixtureRun(t,repo,"-c","user.name=MBVM Test","-c","user.email=mbvm@example.invalid","commit","-m","pax-fixture")
 archiveCmd:=exec.Command("git","-C",repo,"archive","--format=tar","HEAD")
 data,err:=archiveCmd.Output()
 if err!=nil{t.Fatalf("git archive: %v",err)}
 reader:=tar.NewReader(bytes.NewReader(data))
 hasPAX:=false
 for{
  h,err:=reader.Next()
  if err==io.EOF{break}
  if err!=nil{t.Fatal(err)}
  if h.Typeflag==tar.TypeXGlobalHeader||h.Typeflag==tar.TypeXHeader{
   hasPAX=true
  }
 }
 dest:=filepath.Join(t.TempDir(),"extracted")
 if err:=extractGitTree(repo,"HEAD",dest);err!=nil{
  t.Fatalf("git PAX archive extraction failed: %v",err)
 }
 restored,err:=os.ReadFile(filepath.Join(dest,"example.go"))
 if err!=nil||string(restored)!="package example\nfunc Test(){}\n"{
  t.Fatalf("real source not correctly extracted: %q %v",restored,err)
 }
 if _,err:=os.Stat(filepath.Join(dest,"pax_global_header"));!os.IsNotExist(err){
  t.Fatalf("PAX global metadata must never be written as a file: %v",err)
 }
 if !hasPAX{
  t.Log("this Git distribution did not emit a PAX header; direct Windows fixture is still required")
 }else{
  t.Log("verified that Git PAX headers were metadata only and source was extracted")
 }
}

func TestExtractArchiveDeterministicPAXAndRejectLinks(t *testing.T){
 // Go archive/tar.Writer emits a real POSIX global metadata header,
 // independent of Git for Windows version, locale or archive settings.
 const body="package fixture\nfunc Example(){}\n"
 var archive bytes.Buffer
 tw:=tar.NewWriter(&archive)
 if err:=tw.WriteHeader(&tar.Header{
  Name:"pax_global_header",Typeflag:tar.TypeXGlobalHeader,
  PAXRecords:map[string]string{"comment":"metadata-only"},
 });err!=nil{t.Fatal(err)}
 if err:=tw.WriteHeader(&tar.Header{
  Name:"pkg/example.go",Typeflag:tar.TypeReg,Mode:0600,Size:int64(len(body)),
 });err!=nil{t.Fatal(err)}
 if _,err:=tw.Write([]byte(body));err!=nil{t.Fatal(err)}
 if err:=tw.Close();err!=nil{t.Fatal(err)}
 dest:=t.TempDir()
 if err:=extractArchiveEntries(bytes.NewReader(archive.Bytes()),dest);err!=nil{t.Fatal(err)}
 got,err:=os.ReadFile(filepath.Join(dest,"pkg","example.go"))
 if err!=nil||string(got)!=body{t.Fatalf("PAX extraction: got=%q err=%v",got,err)}
 if _,err:=os.Stat(filepath.Join(dest,"pax_global_header"));!os.IsNotExist(err){
  t.Fatalf("PAX metadata was materialized: %v",err)
 }
 // Global metadata must not broaden the extraction policy: symbolic
 // links remain categorically disallowed, including archive escapes.
 archive.Reset()
 tw=tar.NewWriter(&archive)
 if err:=tw.WriteHeader(&tar.Header{
  Name:"bad",Typeflag:tar.TypeSymlink,Linkname:"../../outside",Mode:0777,
 });err!=nil{t.Fatal(err)}
 if err:=tw.Close();err!=nil{t.Fatal(err)}
 if err:=extractArchiveEntries(bytes.NewReader(archive.Bytes()),t.TempDir());err==nil{
  t.Fatal("unsafe TAR symlink accepted after PAX support")
 }
}
