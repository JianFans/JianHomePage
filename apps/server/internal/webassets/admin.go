package webassets

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"golang.org/x/net/html"
)

// validateAdmin also protects binaries compiled from partially deleted build output.
func validateAdmin(tree fs.FS) error {
	if err := validateAdminManifest(tree); err != nil {
		return err
	}
	preview := "preview.html"
	if _, err := fs.Stat(tree, preview); errors.Is(err, fs.ErrNotExist) {
		preview = "preview/index.html"
	}
	for _, entry := range []string{"index.html", preview} {
		data, err := fs.ReadFile(tree, entry)
		if err != nil || len(data) == 0 || len(data) > 4<<20 {
			return fmt.Errorf("embedded management page %q is missing, empty or oversized", entry)
		}
		document, err := html.Parse(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("embedded management page %q is invalid: %w", entry, err)
		}
		scripts := 0
		var visit func(*html.Node) error
		visit = func(node *html.Node) error {
			if node.Type == html.ElementNode {
				attributes := make(map[string]string, len(node.Attr))
				for _, attribute := range node.Attr {
					attributes[attribute.Key] = attribute.Val
				}
				switch node.Data {
				case "base":
					return errors.New("embedded management page contains a base override")
				case "script":
					if source, ok := attributes["src"]; ok {
						if err := requireAdminFile(tree, source, ".js"); err != nil {
							return err
						}
						scripts++
					}
				case "link":
					relations := " " + strings.Join(strings.Fields(strings.ToLower(attributes["rel"])), " ") + " "
					source := attributes["href"]
					pathname, _, _ := strings.Cut(source, "?")
					pathname, _, _ = strings.Cut(pathname, "#")
					extension := path.Ext(pathname)
					if strings.Contains(relations, " stylesheet ") || attributes["as"] == "style" {
						extension = ".css"
					} else if strings.Contains(relations, " modulepreload ") || attributes["as"] == "script" {
						extension = ".js"
					}
					if extension == ".js" || extension == ".css" || extension == ".mjs" {
						if err := requireAdminFile(tree, source, extension); err != nil {
							return err
						}
					}
				}
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if err := visit(child); err != nil {
					return err
				}
			}
			return nil
		}
		if err := visit(document); err != nil {
			return fmt.Errorf("embedded management page %q: %w", entry, err)
		}
		if scripts == 0 {
			return fmt.Errorf("embedded management page %q has no entry script", entry)
		}
	}
	return nil
}

// requireAdminFile accepts root-relative and document-relative Nuxt build URLs.
func requireAdminFile(tree fs.FS, source, extension string) error {
	name := strings.TrimPrefix(strings.TrimPrefix(source, "./"), "/")
	if !strings.HasPrefix(name, "_nuxt/") || !safeBrowserPath(name) || strings.Contains(name, "%") || path.Ext(name) != extension {
		return errors.New("unsafe management browser asset reference")
	}
	info, err := fs.Stat(tree, name)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("management browser dependency %q is missing or empty", name)
	}
	return nil
}
