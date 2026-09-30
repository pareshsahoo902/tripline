package shell

import (
	"reflect"
	"testing"
)

func TestUnknown(t *testing.T) {
	for cmd, want := range map[string]bool{
		"rm -rf build && git stash": true,
		"rm -rf build":              false,
		"git checkout main":         true,
		"echo x > a && patch < d":   true,
		`bash -c "git pull"`:        true,
	} {
		if _, unknown := Writes(cmd); unknown != want {
			t.Errorf("%q: unknown %v, want %v", cmd, unknown, want)
		}
	}
}

func TestWrites(t *testing.T) {
	for _, tc := range []struct {
		cmd    string
		files  []string
		writes bool
	}{
		// Read-only commands.
		{"ls -la", nil, false},
		{"cat main.go | grep foo", nil, false},
		{"go test ./... 2>&1", nil, false},
		{"npm test > /dev/null 2>&1", nil, false},
		{"Get-ChildItem -Force 2>$null", nil, false},
		{"git status && git diff", nil, false},
		{"git reset HEAD~1", nil, false},
		{"sed -n 1,20p main.go", nil, false},
		{"echo '> not a redirect'", nil, false},
		{"# rm -rf build", nil, false},
		{"grep -rn 'a>b' .", nil, false},

		// Redirects.
		{"echo hi > out.txt", []string{"out.txt"}, true},
		{"echo hi>out.txt", []string{"out.txt"}, true},
		{"go test ./... 2> errors.log", []string{"errors.log"}, true},
		{"cat a >> b", []string{"b"}, true},
		{"make &> build.log", []string{"build.log"}, true},
		{`echo x > "my file.txt"`, []string{"my file.txt"}, true},
		{"cat <<'EOF' > conf.yaml\nkey: value > 3\nEOF\nls", []string{"conf.yaml"}, true},

		// File commands.
		{"sed -i 's/a/b/' main.go util.go", []string{"main.go", "util.go"}, true},
		{"sed -i.bak -e 's/a/b/' main.go", []string{"main.go"}, true},
		{"sed -ri 's/a/b/' main.go", []string{"main.go"}, true},
		{"perl -pi -e 's/a/b/' x.txt", []string{"x.txt"}, true},
		{"tee -a log.txt", []string{"log.txt"}, true},
		{"rm -f a.o b.o", []string{"a.o", "b.o"}, true},
		{"mv old.go new.go", []string{"old.go", "new.go"}, true},
		{"cp -r src/ dst/", []string{"dst/"}, true},
		{"touch README.md", []string{"README.md"}, true},
		{"dd if=/dev/zero of=disk.img bs=1M count=1", []string{"disk.img"}, true},
		{"patch -p1 < fix.diff", nil, true},
		{"find . -name '*.tmp' -delete", nil, true},
		{"find . -name '*.tmp' | xargs rm", nil, true},

		// git.
		{"git checkout -- main.go", []string{"main.go"}, true},
		{"git checkout feature", nil, true},
		{"git restore main.go", []string{"main.go"}, true},
		{"git -C sub reset --hard", nil, true},
		{"git stash pop", nil, true},
		{"git mv a.go b.go", []string{"a.go", "b.go"}, true},
		{"git rm --cached secret.txt", nil, false},

		// Formatters and package managers.
		{"gofmt -w main.go", []string{"main.go"}, true},
		{"gofmt -l .", nil, false},
		{"prettier --write src/app.ts", []string{"src/app.ts"}, true},
		{"black --check .", nil, false},
		{"ruff check --fix app.py", []string{"app.py"}, true},
		{"npm install lodash", []string{"package.json", "package-lock.json"}, true},
		{"npm run build", nil, false},
		{"go mod tidy", []string{"go.mod", "go.sum"}, true},

		// cd, wrappers and nested shells.
		{"cd web && sed -i 's/a/b/' app.js", []string{"web/app.js"}, true},
		{"cd /repo && touch /tmp/x", []string{"/tmp/x"}, true},
		{"sudo -u root tee /etc/hosts", []string{"/etc/hosts"}, true},
		{"FOO=1 BAR=2 touch x", []string{"x"}, true},
		{`bash -lc "echo hi > out.txt"`, []string{"out.txt"}, true},
		{"(cd sub; rm -f x)", []string{"sub/x"}, true},

		// PowerShell and cmd.
		{`Set-Content -Path C:\repo\a.txt -Value "hi"`, []string{`C:\repo\a.txt`}, true},
		{`Set-Content a.txt "hi"`, []string{"a.txt"}, true},
		{`Remove-Item -Recurse -Force .\dist`, []string{`.\dist`}, true},
		{`Copy-Item a.txt -Destination b.txt`, []string{"b.txt"}, true},
		{`Move-Item a.txt b.txt`, []string{"a.txt", "b.txt"}, true},
		{`Out-File -FilePath:log.txt -InputObject $x`, []string{"log.txt"}, true},
		{`powershell -Command "Add-Content notes.md 'x'"`, []string{"notes.md"}, true},
		{`cmd /c "del build.log"`, []string{"build.log"}, true},
	} {
		files, unknown := Writes(tc.cmd)
		if writes := unknown || len(files) > 0; writes != tc.writes || !reflect.DeepEqual(files, tc.files) {
			t.Errorf("%q: got %q %v, want %q %v", tc.cmd, files, writes, tc.files, tc.writes)
		}
	}
}
