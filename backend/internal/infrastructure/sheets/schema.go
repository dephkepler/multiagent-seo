package sheets

import "fmt"

const (
	colCredBase    = "E"
	colLogin       = "F"
	colPassword    = "G"
	colLoginStatus = "H"
	colPlacement   = "I"
	colRights      = "J"

	colEmailURL    = "A"
	colEmailList   = "B"
	colEmailStatus = "C"
)

func colRange(sheet, from, to string) string {
	return fmt.Sprintf("%s!%s:%s", sheet, from, to)
}

func colCell(sheet, col string, row int) string {
	return fmt.Sprintf("%s!%s%d", sheet, col, row)
}
