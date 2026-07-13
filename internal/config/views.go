package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const viewsFileName = ".hardcover-views.toml"

type viewsFile struct {
	Views map[string]string `toml:"views"`
}

func viewsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, viewsFileName), nil
}

func LoadViews() (map[string]string, error) {
	p, err := viewsPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, fmt.Errorf("read %s: %w", p, err)
	}

	var vf viewsFile
	if err := toml.Unmarshal(data, &vf); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}

	if vf.Views == nil {
		vf.Views = make(map[string]string)
	}
	return vf.Views, nil
}

func SaveView(name, query string) error {
	views, err := LoadViews()
	if err != nil {
		return err
	}

	views[name] = query
	return writeViews(views)
}

func DeleteView(name string) error {
	views, err := LoadViews()
	if err != nil {
		return err
	}

	if _, ok := views[name]; !ok {
		return fmt.Errorf("view %q not found", name)
	}

	delete(views, name)
	return writeViews(views)
}

func writeViews(views map[string]string) error {
	p, err := viewsPath()
	if err != nil {
		return err
	}

	vf := viewsFile{Views: views}
	f, err := os.Create(p)
	if err != nil {
		return fmt.Errorf("create %s: %w", p, err)
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(vf); err != nil {
		return fmt.Errorf("encode %s: %w", p, err)
	}

	return nil
}
