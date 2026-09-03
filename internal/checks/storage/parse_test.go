package storage

import (
	"testing"
)

func TestParseMounts(t *testing.T) {
	fixture := []byte(`sysfs /sys sysfs rw,nosuid,nodev,noexec,relatime 0 0
proc /proc proc rw,nosuid,nodev,noexec,relatime 0 0
/dev/nvme0n1p2 / ext4 rw,relatime 0 0
tmpfs /run tmpfs rw,nosuid,nodev 0 0
/dev/sda1 /media/My\040Backup vfat rw,relatime 0 0
//nas.local/share /mnt/share cifs rw 0 0
`)

	entries := parseMounts(fixture)
	if len(entries) != 6 {
		t.Fatalf("parsed %d entries, want 6", len(entries))
	}

	var real []mountEntry
	for _, e := range entries {
		if isRealFilesystem(e) {
			real = append(real, e)
		}
	}
	if len(real) != 3 {
		t.Fatalf("kept %d real filesystems, want 3: %+v", len(real), real)
	}
	if real[1].Mount != "/media/My Backup" {
		t.Errorf("escaped mount point = %q, want %q", real[1].Mount, "/media/My Backup")
	}
	if real[2].FSType != "cifs" {
		t.Errorf("network share was dropped: %+v", real)
	}
}

func TestBlockDeviceIsPhysical(t *testing.T) {
	physical := []string{"sda", "nvme0n1", "vda", "hda"}
	virtual := []string{"loop0", "ram3", "zram0", "dm-1", "sr0", "md0"}

	for _, name := range physical {
		if !blockDeviceIsPhysical(name) {
			t.Errorf("%s was treated as virtual", name)
		}
	}
	for _, name := range virtual {
		if blockDeviceIsPhysical(name) {
			t.Errorf("%s was treated as a physical drive", name)
		}
	}
}

func TestParseSmartctl(t *testing.T) {
	healthy := []byte(`{"device":{"name":"/dev/sda"},"model_name":"Samsung SSD 870",` +
		`"smart_status":{"passed":true},"ata_smart_attributes":{"table":[` +
		`{"id":5,"name":"Reallocated_Sector_Ct","raw":{"value":0}}]}}`)
	d, err := parseSmartctl(healthy, "/dev/sda")
	if err != nil {
		t.Fatalf("parseSmartctl: %v", err)
	}
	if d.Status != statusHealthy || d.Model != "Samsung SSD 870" {
		t.Errorf("healthy disk = %+v", d)
	}
	if d.Reallocated == nil || *d.Reallocated != 0 {
		t.Errorf("reallocated = %v, want 0", d.Reallocated)
	}

	failing := []byte(`{"device":{"name":"/dev/sdb"},"smart_status":{"passed":false},` +
		`"ata_smart_attributes":{"table":[{"id":5,"name":"Reallocated_Sector_Ct","raw":{"value":142}}]}}`)
	d, err = parseSmartctl(failing, "/dev/sdb")
	if err != nil {
		t.Fatalf("parseSmartctl: %v", err)
	}
	if d.Status != statusFailing {
		t.Errorf("status = %q, want failing", d.Status)
	}
	if d.Reallocated == nil || *d.Reallocated != 142 {
		t.Errorf("reallocated = %v, want 142", d.Reallocated)
	}
}

func TestParseSmartctlWithoutAVerdictIsUnknown(t *testing.T) {
	// USB enclosures commonly pass no SMART data through at all.
	enclosure := []byte(`{"device":{"name":"/dev/sdc"},"model_name":"USB Bridge"}`)

	d, err := parseSmartctl(enclosure, "/dev/sdc")
	if err != nil {
		t.Fatalf("parseSmartctl: %v", err)
	}
	if d.Status != statusUnknown {
		t.Errorf("status = %q, want unknown — no verdict must never read as healthy", d.Status)
	}
	if d.Reallocated != nil {
		t.Errorf("reallocated = %v, want absent", *d.Reallocated)
	}
}

func TestParseWindowsVolumes(t *testing.T) {
	fixture := []byte(`[{"DeviceID":"C:","FileSystem":"NTFS","VolumeName":"Windows",` +
		`"Size":"511103954944","FreeSpace":"20044120064"},` +
		`{"DeviceID":"E:","FileSystem":null,"VolumeName":null,"Size":0,"FreeSpace":0}]`)

	volumes, err := parseWindowsVolumes(fixture)
	if err != nil {
		t.Fatalf("parseWindowsVolumes: %v", err)
	}
	if len(volumes) != 1 {
		t.Fatalf("got %d volumes, want 1 (an empty card slot is not a drive)", len(volumes))
	}
	if volumes[0].Mount != "C:" || volumes[0].TotalBytes != 511103954944 {
		t.Errorf("volume = %+v", volumes[0])
	}
	if got := volumes[0].FreePercent(); got < 3.9 || got > 4.0 {
		t.Errorf("free percent = %.2f, want about 3.92", got)
	}
}

func TestParseWindowsDisksMapsHealth(t *testing.T) {
	fixture := []byte(`[{"FriendlyName":"Samsung SSD 980","HealthStatus":"Healthy"},` +
		`{"FriendlyName":"Seagate ST2000","HealthStatus":"Warning"},` +
		`{"FriendlyName":"Old Drive","HealthStatus":"Something new"}]`)

	disks, err := parseWindowsDisks(fixture)
	if err != nil {
		t.Fatalf("parseWindowsDisks: %v", err)
	}
	want := []status{statusHealthy, statusFailing, statusUnknown}
	for i, w := range want {
		if disks[i].Status != w {
			t.Errorf("disk %d status = %q, want %q", i, disks[i].Status, w)
		}
	}
}

func TestApplyFailurePredictions(t *testing.T) {
	disks := []disk{{Name: "Samsung SSD 980", Status: statusHealthy}}

	unchanged := applyFailurePredictions(disks, []byte(`{"InstanceName":"SCSI\\Disk","PredictFailure":false}`))
	if unchanged[0].Status != statusHealthy {
		t.Errorf("status = %q, want healthy when nothing is predicted", unchanged[0].Status)
	}

	flagged := applyFailurePredictions(disks, []byte(`{"InstanceName":"SCSI\\Disk","PredictFailure":true}`))
	if flagged[0].Status != statusFailing {
		t.Errorf("status = %q, want failing when Windows predicts failure", flagged[0].Status)
	}

	// An empty result (no administrator rights) leaves the verdict alone.
	disks[0].Status = statusHealthy
	untouched := applyFailurePredictions(disks, nil)
	if untouched[0].Status != statusHealthy {
		t.Errorf("status = %q, want healthy when the class returns nothing", untouched[0].Status)
	}
}

func TestParseDiskutilInfo(t *testing.T) {
	fixture := []byte(`   Device Identifier:        disk0
   Device / Media Name:      APPLE SSD AP0512Q
   Whole:                    Yes
   SMART Status:             Verified

**********

   Device Identifier:        disk4
   Device / Media Name:      External USB Drive
   Whole:                    Yes
   SMART Status:             Not Supported

**********

   Device Identifier:        disk5
   Device / Media Name:      Failing Drive
   Whole:                    Yes
   SMART Status:             Failing
`)

	disks := parseDiskutilInfo(fixture)
	if len(disks) != 3 {
		t.Fatalf("got %d disks, want 3: %+v", len(disks), disks)
	}
	want := []status{statusHealthy, statusUnknown, statusFailing}
	for i, w := range want {
		if disks[i].Status != w {
			t.Errorf("%s status = %q, want %q", disks[i].Name, disks[i].Status, w)
		}
	}
	if disks[0].Model != "APPLE SSD AP0512Q" {
		t.Errorf("model = %q", disks[0].Model)
	}
}

func TestVolumeFreePercentWithNoSize(t *testing.T) {
	if got := (volume{}).FreePercent(); got != 0 {
		t.Errorf("FreePercent on an empty volume = %v, want 0", got)
	}
}
