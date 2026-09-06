package devtool

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PackageOptions drive Package.
type PackageOptions struct {
	RootDir string
	Env     Env
	Stdout  io.Writer
}

// Package writes dist/release/loaf_<version>_<target>.tar.gz for every
// requested target plus checksums.txt, the assets GitHub Releases, Homebrew,
// and install.sh consume. Each archive is a complete distribution: the native
// binary at bin/<name>, the manifest, config, content, the authored vnext
// Flow content, the built dist and plugin trees, and the Claude Code
// marketplace manifest.
func Package(options PackageOptions) error {
	root := options.RootDir
	stdout := options.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	version, err := manifestVersion(root)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, "vnext", "content", "skills")); err != nil {
		return fmt.Errorf("missing tracker-native Flow content at vnext/content/skills.")
	}
	targets, err := ReleaseTargetsFromEnv(options.Env)
	if err != nil {
		return err
	}
	outDir := filepath.Join(root, "dist", "release")
	if err := os.RemoveAll(outDir); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	var checksums []string
	for _, target := range targets {
		nativeSource := filepath.Join(root, "bin", "native", target.RuntimeID, target.BinaryName())
		if _, err := os.Stat(nativeSource); err != nil {
			return fmt.Errorf("missing native binary for %s: %s\nRun `make release` before packaging release archives.", target.RuntimeID, nativeSource)
		}
		packageName := "loaf_" + version + "_" + target.RuntimeID
		archivePath := filepath.Join(outDir, packageName+".tar.gz")
		digest, err := writeReleaseArchive(root, archivePath, packageName, nativeSource, target.BinaryName())
		if err != nil {
			return err
		}
		checksums = append(checksums, digest+"  "+packageName+".tar.gz")
		fmt.Fprintf(stdout, "✓ Packaged %s.tar.gz\n", packageName)
	}
	checksumsPath := filepath.Join(outDir, "checksums.txt")
	if err := os.WriteFile(checksumsPath, []byte(strings.Join(checksums, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "✓ Wrote %s\n", checksumsPath)
	return nil
}

func manifestVersion(root string) (string, error) {
	body, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return "", fmt.Errorf("read package.json: %w", err)
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		return "", fmt.Errorf("parse package.json: %w", err)
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return "", fmt.Errorf("package.json missing version.")
	}
	return manifest.Version, nil
}

// archiveEntries are the distribution parts, in the order they are written.
var archiveFiles = []string{"package.json", "README.md", "CHANGELOG.md"}
var archiveDirs = []string{"config", "content", "vnext/content", "dist", "plugins", ".claude-plugin"}

func writeReleaseArchive(root, archivePath, packageName, nativeSource, binaryName string) (string, error) {
	file, err := os.Create(archivePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	gz := gzip.NewWriter(io.MultiWriter(file, hasher))
	tw := tar.NewWriter(gz)

	addFile := func(source, name string, mode int64) error {
		info, err := os.Stat(source)
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: info.Size(), ModTime: info.ModTime(), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		src, err := os.Open(source)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(tw, src)
		return err
	}
	if err := addFile(nativeSource, packageName+"/bin/"+binaryName, 0o755); err != nil {
		return "", err
	}
	for _, rel := range archiveFiles {
		source := filepath.Join(root, rel)
		if _, err := os.Stat(source); err != nil {
			continue
		}
		if err := addFile(source, packageName+"/"+rel, 0o644); err != nil {
			return "", err
		}
	}
	for _, dir := range archiveDirs {
		source := filepath.Join(root, filepath.FromSlash(dir))
		if _, err := os.Stat(source); err != nil {
			continue
		}
		var paths []string
		err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				// The release output itself never nests into an archive.
				if dir == "dist" && path == filepath.Join(source, "release") {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Name() == ".DS_Store" {
				return nil
			}
			paths = append(paths, path)
			return nil
		})
		if err != nil {
			return "", err
		}
		sort.Strings(paths)
		for _, path := range paths {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return "", err
			}
			info, err := os.Lstat(path)
			if err != nil {
				return "", err
			}
			mode := int64(0o644)
			if info.Mode().Perm()&0o111 != 0 {
				mode = 0o755
			}
			if err := addFile(path, packageName+"/"+filepath.ToSlash(rel), mode); err != nil {
				return "", err
			}
		}
	}
	if err := tw.Close(); err != nil {
		return "", err
	}
	if err := gz.Close(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}
