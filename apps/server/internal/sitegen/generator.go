// Package sitegen renders the shared Vue homepage inside the Go process.
// Only the application-owned bundle executes; content is passed as JSON data.
package sitegen

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/dop251/goja"
	weburl "github.com/nlnwa/whatwg-url/url"
	"yujian.me/server/internal/contract"
)

const maxSnapshot = 8 << 20
const maxOutput = 32 << 20

// Generator retains immutable compiled code and assets, never a shared JS VM.
type Generator struct {
	program  *goja.Program
	files    map[string][]byte
	identity string
}

// New compiles trusted build output and validates embedded public file limits.
func New(renderer []byte, assets fs.FS) (*Generator, error) {
	if len(renderer) == 0 || len(renderer) > maxOutput || assets == nil {
		return nil, errors.New("missing or oversized site resources")
	}
	program, err := goja.Compile("renderer.js", string(renderer), false)
	if err != nil {
		return nil, errors.New("invalid bundled site renderer")
	}
	files := make(map[string][]byte)
	total := 0
	err = fs.WalkDir(assets, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") && name != "." {
				return fs.SkipDir
			}
			return nil
		}
		if !fs.ValidPath(name) || entry.Type()&fs.ModeSymlink != 0 {
			return errors.New("unsafe public asset")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxOutput || total+int(info.Size()) > 128<<20 {
			return errors.New("public assets exceed size limit")
		}
		data, err := fs.ReadFile(assets, name)
		if err != nil {
			return err
		}
		total += len(data)
		files[name] = data
		return nil
	})
	if err != nil || len(files) == 0 {
		return nil, errors.New("invalid or missing public assets")
	}
	hash := sha256.New()
	hash.Write(renderer)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(hash, "\x00%s\x00", name)
		hash.Write(files[name])
	}
	return &Generator{program: program, files: files, identity: hex.EncodeToString(hash.Sum(nil))}, nil
}

// Identity freezes the renderer and browser assets as one deterministic build identity.
func (generator *Generator) Identity() string { return generator.identity }

// Generate validates content then renders in a fresh, time-limited JS VM with no
// filesystem, network, module loader or subprocess APIs.
func (generator *Generator) Generate(parent context.Context, raw []byte) (map[string][]byte, error) {
	if len(raw) == 0 || len(raw) > maxSnapshot {
		return nil, errors.New("snapshot exceeds size limit")
	}
	if err := contract.NewValidator().Validate(raw); err != nil {
		return nil, fmt.Errorf("invalid content snapshot: %w", err)
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	vm := goja.New()
	vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))
	vm.SetMaxCallStackSize(512)
	stop := context.AfterFunc(ctx, func() { vm.Interrupt(ctx.Err()) })
	defer stop()
	if err := installBrowserPrimitives(vm); err != nil {
		return nil, err
	}
	if _, err := vm.RunProgram(generator.program); err != nil {
		return nil, fmt.Errorf("initialize bundled renderer: %w", err)
	}
	render, ok := goja.AssertFunction(vm.Get("__render"))
	if !ok {
		return nil, errors.New("bundled renderer has no entry point")
	}
	result, err := render(goja.Undefined(), vm.ToValue(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("render homepage: %w", err)
	}
	promise, ok := result.Export().(*goja.Promise)
	if ok && promise.State() == goja.PromiseStateRejected {
		return nil, fmt.Errorf("bundled renderer failed: %s", promise.Result().String())
	}
	if !ok || promise.State() != goja.PromiseStateFulfilled {
		return nil, errors.New("bundled renderer did not complete")
	}
	var document struct {
		HTML    string `json:"html"`
		Robots  string `json:"robots"`
		Sitemap string `json:"sitemap"`
	}
	if err := vm.ExportTo(promise.Result(), &document); err != nil {
		return nil, errors.New("invalid renderer output")
	}
	if len(document.HTML) == 0 || len(document.HTML)+len(document.Robots)+len(document.Sitemap) > maxOutput {
		return nil, errors.New("rendered document exceeds size limit")
	}
	files := make(map[string][]byte, len(generator.files)+3)
	for name, data := range generator.files {
		files[name] = append([]byte(nil), data...)
	}
	files["index.html"] = []byte(document.HTML)
	files["robots.txt"] = []byte(document.Robots)
	files["sitemap.xml"] = []byte(document.Sitemap)
	return files, ctx.Err()
}

// installBrowserPrimitives supplies only the URL fields and UTC date format
// used by the shared components. It grants no I/O capability to the renderer.
func installBrowserPrimitives(vm *goja.Runtime) error {
	if err := vm.Set("__url", func(value, base string) map[string]string {
		var parsed *weburl.Url
		var err error
		if base != "undefined" && base != "" {
			parsed, err = weburl.ParseRef(base, value)
		} else {
			parsed, err = weburl.Parse(value)
		}
		if err != nil {
			panic(vm.NewTypeError("invalid URL"))
		}
		return map[string]string{"href": parsed.Href(false), "hostname": parsed.Hostname(), "protocol": parsed.Protocol()}
	}); err != nil {
		return err
	}
	if err := vm.Set("__date", func(locale string, milliseconds float64) string {
		value := time.UnixMilli(int64(milliseconds)).UTC()
		if locale == "en" {
			return value.Format("Jan 02")
		}
		return fmt.Sprintf("%d月%02d日", value.Month(), value.Day())
	}); err != nil {
		return err
	}
	_, err := vm.RunString(`globalThis.console = { warn(...args) { throw new Error(args.join(' ')); }, error(...args) { throw new Error(args.join(' ')); } }; globalThis.URL = class URL { constructor(value, base) { Object.assign(this, __url(String(value), String(base))); } }; globalThis.Intl = { DateTimeFormat: class DateTimeFormat { constructor(locale) { this.locale = locale; } format(value) { return __date(this.locale, Number(value)); } } };`)
	return err
}
