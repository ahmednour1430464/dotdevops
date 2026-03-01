package main
import "fmt"
func main() {
	var tName string
	n, err := fmt.Sscanf("unknown type \"package.install\" (not a primitive or defined step)", "unknown type %q", &tName)
	fmt.Printf("n=%d, err=%v, tName=%q\n", n, err, tName)
}
