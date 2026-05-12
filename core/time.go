package core

import "time"

var shanghaiLoc *time.Location

func init() {
	var err error
	shanghaiLoc, err = time.LoadLocation("Asia/Shanghai")
	if err != nil {
		shanghaiLoc = time.FixedZone("CST", 8*3600)
	}
}

func Now() time.Time {
	return time.Now().In(shanghaiLoc)
}
