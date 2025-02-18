// Copyright 2019 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !race
// +build !race

package integration

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hugelgupf/vmtest"
	"github.com/hugelgupf/vmtest/qemu"
	"github.com/u-root/u-root/pkg/boot/multiboot"
	"github.com/u-root/u-root/pkg/uroot"
)

func testMultiboot(t *testing.T, kernel string) {
	src := filepath.Join(os.Getenv("UROOT_MULTIBOOT_TEST_KERNEL_DIR"), kernel)
	if _, err := os.Stat(src); err != nil && os.IsNotExist(err) {
		t.Skip("multiboot kernel is not present")
	} else if err != nil {
		t.Error(err)
	}

	dir := t.TempDir()
	testCmds := []string{
		`kexec -l kernel -e -d --module="/kernel foo=bar" --module="/bbin/bb" | tee /testdata/output.json`,
	}
	vm := vmtest.StartVMAndRunCmds(t, testCmds,
		vmtest.WithSharedDir(dir),
		vmtest.WithMergedInitramfs(uroot.Opts{
			Commands: uroot.BusyBoxCmds(
				"github.com/u-root/u-root/cmds/core/kexec",
				"github.com/u-root/u-root/cmds/core/tee",
			),
			ExtraFiles: []string{
				src + ":kernel",
			},
		}),
		vmtest.WithQEMUFn(
			qemu.WithVMTimeout(time.Minute),
		),
	)

	if _, err := vm.Console.ExpectString(`"status": "ok"`); err != nil {
		t.Errorf(`expected '"status": "ok"', got error: %v`, err)
	}
	if _, err := vm.Console.ExpectString(`}`); err != nil {
		t.Errorf(`expected '}' = end of JSON, got error: %v`, err)
	}
	if err := vm.Wait(); err != nil {
		t.Fatal(err)
	}

	output, err := os.ReadFile(filepath.Join(dir, "output.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(output))

	i := bytes.Index(output, []byte(multiboot.DebugPrefix))
	if i == -1 {
		t.Fatalf("%q prefix not found in output", multiboot.DebugPrefix)
	}
	output = output[i+len(multiboot.DebugPrefix):]
	if i = bytes.Index(output, []byte{'\n'}); i == -1 {
		t.Fatalf("Cannot find newline character")
	}
	var want multiboot.Description
	if err := json.Unmarshal(output[:i], &want); err != nil {
		t.Fatalf("Cannot unmarshal multiboot debug information: %v", err)
	}

	const starting = "Starting multiboot kernel"
	if i = bytes.Index(output, []byte(starting)); i == -1 {
		t.Fatalf("Multiboot kernel was not executed")
	}
	output = output[i+len(starting):]

	var got multiboot.Description
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("Cannot unmarshal multiboot information from executed kernel: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("kexec failed: got\n%#v, want\n%#v", got, want)
	}
}

func TestMultiboot(t *testing.T) {
	vmtest.SkipIfNotArch(t, qemu.ArchAMD64, qemu.ArchArm64)

	for _, kernel := range []string{"/kernel", "/kernel.gz"} {
		t.Run(kernel, func(t *testing.T) {
			testMultiboot(t, kernel)
		})
	}
}
