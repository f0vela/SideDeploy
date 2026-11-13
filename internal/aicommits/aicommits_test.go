package aicommits

import (
	"context"
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Añade paginación al listado", "Añade paginación al listado"},
		{"  \n Añade paginación \n\n", "Añade paginación"},
		{"```\nAñade paginación\n```", "Añade paginación"},
		{"```text\nfeat: añade paginación\n\n- Detalle\n```", "feat: añade paginación\n\n- Detalle"},
		{"\"Añade paginación\"", "Añade paginación"},
		{"'Añade paginación'", "Añade paginación"},
		{"Mensaje: Añade paginación", "Añade paginación"},
		{"mensaje del commit:\nAñade paginación", "Añade paginación"},
		{"Commit message: Añade paginación", "Añade paginación"},
		{"Mensaje:\n\nAñade paginación", "Añade paginación"},
		{"Mensaje: \"Añade paginación\"", "Añade paginación"},
		{"", ""},
	}
	for _, c := range cases {
		if got := Clean(c.in); got != c.want {
			t.Errorf("Clean(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGenerateGuards(t *testing.T) {
	ctx := context.Background()
	if _, err := Generate(ctx, "claude", "", "  \n", ""); err == nil {
		t.Fatal("an empty diff was accepted")
	}
	if _, err := Generate(ctx, "no-such-tool-sidedeploy", "", "diff", ""); err == nil ||
		!strings.Contains(err.Error(), "PATH") {
		t.Fatalf("missing command error = %v", err)
	}
}

func TestResolve(t *testing.T) {
	if bin, err := Resolve("sh", ""); err != nil || !strings.HasSuffix(bin, "/sh") {
		t.Fatalf("Resolve(sh) = %q, %v", bin, err)
	}
	if _, err := Resolve("no-such-tool-sidedeploy", ""); err == nil ||
		!strings.Contains(err.Error(), "PATH") {
		t.Fatalf("missing tool error = %v", err)
	}
	if _, err := Resolve("claude", "/no/such/claude"); err == nil ||
		!strings.Contains(err.Error(), "/no/such/claude") {
		t.Fatalf("missing configured path error = %v", err)
	}
	if _, err := Resolve("claude", t.TempDir()); err == nil {
		t.Fatal("a directory was accepted as an executable")
	}
	if bin, err := Resolve("claude", "/bin/sh"); err != nil || bin != "/bin/sh" {
		t.Fatalf("configured path = %q, %v", bin, err)
	}
}

func TestPrompt(t *testing.T) {
	p := strings.ToLower(prompt("+uno", "feat: algo"))
	for _, want := range []string{"mensajes recientes", "feat: algo", "+uno", "72"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q:\n%s", want, p)
		}
	}
}
