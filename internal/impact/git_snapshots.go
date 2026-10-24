package impact

import (
	"archive/tar"
	"context"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const maxSnapshotBytes int64 = 256 << 20

// CompareGitRefs reads two LOCAL committed Git trees via git archive.
// Only temporary snapshots are extracted; it does not checkout, stash,
// reset, update git refs, or change the working directory's tracked files.
func CompareGitRefs(repoRoot, oldRef, newRef, packageRel string) (VersionReport, error) {
	var empty VersionReport
	if oldRef == "" || newRef == "" || strings.HasPrefix(oldRef, "-") || strings.HasPrefix(newRef, "-") {
		return empty, fmt.Errorf("two valid local Git refs are required")
	}
	cleanPackage := filepath.Clean(packageRel)
	if !filepath.IsLocal(cleanPackage) && cleanPackage != "." {
		return empty, fmt.Errorf("package path must be relative and contained in the repository")
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return empty, err
	}
	snapshots, err := os.MkdirTemp("", "mbvm-impact-git-")
	if err != nil {
		return empty, err
	}
	defer func() { _ = os.RemoveAll(snapshots) }()

	oldRoot := filepath.Join(snapshots, "old")
	newRoot := filepath.Join(snapshots, "new")
	if err = extractGitTree(root, oldRef, oldRoot); err != nil {
		return empty, fmt.Errorf("old revision %q: %w", oldRef, err)
	}
	if err = extractGitTree(root, newRef, newRoot); err != nil {
		return empty, fmt.Errorf("new revision %q: %w", newRef, err)
	}
	return CompareDirs(filepath.Join(oldRoot, cleanPackage), filepath.Join(newRoot, cleanPackage))
}

// boundedArchiveWriter rejects excess tar-stream bytes, including tar
// headers/padding. It is used by os/exec.Run, which manages its own I/O
// copying and waits for that copying to finish before returning.
// This avoids the StdoutPipe + Wait synchronization problem on Windows.
type boundedArchiveWriter struct {
 dest io.Writer
 remaining int64
}
func (w *boundedArchiveWriter) Write(p []byte) (int,error) {
 if int64(len(p))>w.remaining {
  return 0,fmt.Errorf("Git tar stream exceeds %d-byte limit",maxSnapshotBytes)
 }
 n,err:=w.dest.Write(p)
 w.remaining-=int64(n)
 return n,err
}

func extractGitTree(repo,ref,dest string)error{
 if err:=os.MkdirAll(dest,0700);err!=nil{return err}
 // A regular temporary file avoids keeping the archive in memory. The
 // writer caps the entire stream BEFORE tar extraction, so an overlarge
 // input cannot consume unlimited disk or trigger a blocked pipe reader.
 tmp,err:=os.CreateTemp(filepath.Dir(dest),"mbvm-git-archive-*.tar")
 if err!=nil{return err}
 defer os.Remove(tmp.Name())
 defer tmp.Close()

 ctx,cancel:=context.WithTimeout(context.Background(),2*time.Minute)
 defer cancel()
 cmd:=exec.CommandContext(ctx,"git","-C",repo,"archive","--format=tar",ref)
 var stderr bytes.Buffer
 cmd.Stdout=&boundedArchiveWriter{dest:tmp,remaining:maxSnapshotBytes}
 cmd.Stderr=&stderr
 // cmd.Run synchronizes command exit AND its managed copy goroutines;
 // never call Wait with an undrained child stdout pipe.
 if err:=cmd.Run();err!=nil{
  if ctx.Err()!=nil{return fmt.Errorf("git archive timed out: %w",ctx.Err())}
  return fmt.Errorf("git archive failed: %w: %s",err,strings.TrimSpace(stderr.String()))
 }
 if _,err:=tmp.Seek(0,io.SeekStart);err!=nil{return err}
 reader:=tar.NewReader(tmp)
 var total int64
 for{
  h,err:=reader.Next()
  if err==io.EOF{break}
  if err!=nil{return fmt.Errorf("read git archive: %w",err)}
  name:=filepath.Clean(filepath.FromSlash(h.Name))
  if !filepath.IsLocal(name)||name=="."{
   return fmt.Errorf("unsafe archive path %q",h.Name)
  }
  output:=filepath.Join(dest,name)
  switch h.Typeflag {
  case tar.TypeXGlobalHeader,tar.TypeXHeader:
   // PAX metadata extends TAR headers; Go's tar.Reader has already
   // parsed the record. It is metadata only: never create a file for it.
   continue
  case tar.TypeDir:
   if err:=os.MkdirAll(output,0700);err!=nil{return err}
  case tar.TypeReg,tar.TypeRegA:
   if h.Size<0||h.Size>maxSnapshotBytes-total{
    return fmt.Errorf("Git snapshot extracted contents exceed %d-byte limit",maxSnapshotBytes)
   }
   total+=h.Size
   if err:=os.MkdirAll(filepath.Dir(output),0700);err!=nil{return err}
   fd,err:=os.OpenFile(output,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600)
   if err!=nil{return err}
   _,readErr:=io.CopyN(fd,reader,h.Size)
   closeErr:=fd.Close()
   if readErr!=nil{return readErr}
   if closeErr!=nil{return closeErr}
  default:
   // Never materialize links or special files in a Git snapshot.
   return fmt.Errorf("unsupported Git archive entry %q type %d",h.Name,h.Typeflag)
  }
 }
 return nil
}
