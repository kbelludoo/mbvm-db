package impact

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func extractGitTree(repo, ref, dest string) error {
	if err := os.MkdirAll(dest, 0700); err != nil {
		return err
	}
	cmd := exec.Command("git", "-C", repo, "archive", "--format=tar", ref)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var errors bytes.Buffer
	cmd.Stderr = &errors
	if err = cmd.Start(); err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	reader := tar.NewReader(stdout)
	total := int64(0)
	for {
		h, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read git archive: %w", err)
		}
		name := filepath.Clean(filepath.FromSlash(h.Name))
		if !filepath.IsLocal(name) || name == "." {
			return fmt.Errorf("unsafe archive path %q", h.Name)
		}
		output := filepath.Join(dest, name)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(output, 0700); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if h.Size < 0 || h.Size > maxSnapshotBytes-total {
				return fmt.Errorf("Git snapshot exceeds %d-byte limit", maxSnapshotBytes)
			}
			total += h.Size
			if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
				return err
			}
			fd, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			_, readErr := io.CopyN(fd, reader, h.Size)
			closeErr := fd.Close()
			if readErr != nil {
				return readErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("unsupported Git archive entry %q type %d", h.Name, h.Typeflag)
		}
	}
	_, _ = io.Copy(io.Discard, stdout)
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("git archive failed: %w: %s", err, strings.TrimSpace(errors.String()))
	}
	complete = true
	return nil
}
