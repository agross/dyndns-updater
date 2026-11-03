package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type profileState struct {
	Identity string    `json:"identity"`
	Address  string    `json:"address"`
	Updated  time.Time `json:"updated"`
	Failing  bool      `json:"failing"`
}
type diskState struct {
	Version  int                     `json:"version"`
	Profiles map[string]profileState `json:"profiles"`
	Dirty    bool                    `json:"dirty"`
	Pending  []string                `json:"pending"`
}

func lockState(path string) (*os.File, error) {
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.New("cannot open state lock (create state directory first)")
	}
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		f.Close()
		return nil, errors.New("cannot lock state")
	}
	return f, nil
}
func readState(path string) (diskState, error) {
	s := diskState{Version: 1, Profiles: map[string]profileState{}}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, errors.New("cannot read state; preserved")
	}
	defer f.Close()
	content, err := io.ReadAll(io.LimitReader(f, (8<<20)+1))
	if err != nil || len(content) > 8<<20 {
		return s, errors.New("state unreadable or exceeds 8 MiB; preserved")
	}
	s = diskState{}
	dec := json.NewDecoder(bytes.NewReader(content))
	dec.DisallowUnknownFields()
	if dec.Decode(&s) != nil || s.Version != 1 || s.Profiles == nil {
		return s, errors.New("invalid state; preserved")
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return s, errors.New("invalid state; preserved")
	}
	for _, p := range s.Profiles {
		if p.Identity == "" || (p.Address != "" && (net.ParseIP(p.Address) == nil || net.ParseIP(p.Address).To4() == nil || p.Updated.IsZero())) || (p.Address == "" && !p.Updated.IsZero()) {
			return s, errors.New("invalid profile state; preserved")
		}
	}
	return s, nil
}
func saveState(path string, s diskState) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return errors.New("cannot encode state")
	}
	if len(b) > 8<<20 {
		return errors.New("state exceeds 8 MiB; previous state preserved (check pending mail backlog)")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".dyndns-state-*")
	if err != nil {
		return errors.New("cannot create state temporary file")
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err == nil {
		dir, e := os.Open(filepath.Dir(path))
		if e != nil {
			err = e
		} else {
			err = dir.Sync()
			dir.Close()
		}
	}
	if err != nil {
		return errors.New("cannot persist state")
	}
	return nil
}
