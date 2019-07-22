package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"image/png"
	"io"
	"log"
	"math"
	"net"
	"os"

	"image"

	"github.com/apparentlymart/go-cidr/cidr"
)

type Cidr net.IPNet

// UnmarshalText implements the encoding.TextUnmarshaler interface.
// The IP address is expected in a form accepted by ParseIP.
func (c *Cidr) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		return errors.New("Cannot unmarshal cidr from empty string")
	}
	s := string(text)
	_, net, err := net.ParseCIDR(s)
	if err != nil {
		return err
	}
	*c = Cidr(*net)
	return nil
}

func (c Cidr) Net() *net.IPNet {
	n := net.IPNet(c)
	return &n
}

func (c Cidr) String() string {
	return c.Net().String()
}

type VPC struct {
	Vpc     string
	Name    string
	Cidr    Cidr
	Subnets []Subnet
}

var _ CidrBlockInfo = VPC{}

func (v VPC) ForEachIP(callback func(int, net.IP)) {
	network := v.Cidr.Net()
	start, _ := cidr.AddressRange(network)

	for ip, i := start, 0; network.Contains(ip); ip, i = cidr.Inc(ip), i+1 {
		callback(i, ip)
	}
}

func (v VPC) IPAllocated(ip net.IP) bool {
	for _, sub := range v.Subnets {
		if sub.Cidr.Net().Contains(ip) {
			return sub.IPAllocated(ip)
		}
	}

	return false
}

func (v VPC) IPReserved(ip net.IP) bool {
	for _, sub := range v.Subnets {
		if sub.Cidr.Net().Contains(ip) {
			return sub.IPReserved(ip)
		}
	}

	return false
}

func (v VPC) IPInUse(ip net.IP) bool {
	for _, sub := range v.Subnets {
		if sub.Cidr.Net().Contains(ip) {
			return sub.IPInUse(ip)
		}
	}

	return false
}

type CidrBlockInfo interface {
	ForEachIP(func(int, net.IP))
	IPAllocated(net.IP) bool
	IPReserved(net.IP) bool
	IPInUse(net.IP) bool
}

type Subnet struct {
	Cidr    Cidr
	Vpc     string
	Name    string
	Az      string
	UsedIps []net.IP
}

var _ CidrBlockInfo = Subnet{}

func (s Subnet) ForEachIP(callback func(int, net.IP)) {
	network := s.Cidr.Net()
	start, _ := cidr.AddressRange(network)

	for ip, i := start, 0; network.Contains(ip); ip, i = cidr.Inc(ip), i+1 {
		callback(i, ip)
	}
}

func (s Subnet) IPAllocated(ip net.IP) bool {
	return true
}

func (s Subnet) IPReserved(ip net.IP) bool {
	var (
		networkAddr      = s.Cidr.Net().IP
		gatewayAddr      = cidr.Inc(networkAddr)
		dnsAddr          = cidr.Inc(gatewayAddr)
		reserved         = cidr.Inc(dnsAddr)
		_, broadcastAddr = cidr.AddressRange(s.Cidr.Net())
	)
	return ip.Equal(networkAddr) ||
		ip.Equal(gatewayAddr) ||
		ip.Equal(dnsAddr) ||
		ip.Equal(reserved) ||
		ip.Equal(broadcastAddr)
}

func (s Subnet) IPInUse(ip net.IP) bool {

	for _, used := range s.UsedIps {
		if ip.Equal(used) {
			return true
		}
	}

	return false
}

func main() {
	dec := json.NewDecoder(os.Stdin)

	for {
		var vpc VPC
		err := dec.Decode(&vpc)
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}

		fmt.Println("---------------------")
		fmt.Println(vpc.Vpc)
		fmt.Println(vpc.Cidr)
		var totalAllocated uint64 = 0
		var totalReserved uint64 = 0
		var totalUsed uint64 = 0
		var totalVpcIps uint64 = cidr.AddressCount(vpc.Cidr.Net())
		for _, subnet := range vpc.Subnets {
			var (
				networkAddr      = subnet.Cidr.IP
				gatewayAddr      = cidr.Inc(networkAddr)
				dnsAddr          = cidr.Inc(gatewayAddr)
				reserved         = cidr.Inc(dnsAddr)
				_, broadcastAddr = cidr.AddressRange(subnet.Cidr.Net())
			)

			totalReserved += 5
			fmt.Printf("%s\n", subnet.Name)
			fmt.Printf("Cidr Block:   % 18s\n", subnet.Cidr)
			fmt.Printf("Available IPs: %d\n", cidr.AddressCount(subnet.Cidr.Net())-5)
			fmt.Printf("Used IPs: %d\n", len(subnet.UsedIps))
			fmt.Printf("Reserved IPs: % 15s, % 15s, % 15s, % 15s, % 15s\n", networkAddr, gatewayAddr, dnsAddr, reserved, broadcastAddr)

			totalUsed += uint64(len(subnet.UsedIps))
			totalAllocated += cidr.AddressCount(subnet.Cidr.Net())

		}

		fmt.Printf("Total IPs: %d\n", totalVpcIps)
		fmt.Printf("Total Unallocated IPs: %d\n", totalVpcIps-totalAllocated)
		fmt.Printf("Total Allocated IPs: %d\n", totalAllocated)
		fmt.Printf("Total Available IPs: %d\n", totalAllocated-(totalReserved+totalUsed))
		fmt.Printf("Total Reserved  IPs: %d\n", totalReserved)
		fmt.Printf("Total Used      IPs: %d\n", totalUsed)

		generateImage(vpc.Name, vpc.Cidr, vpc)
	}
}

func generateImage(name string, network Cidr, info CidrBlockInfo) {
	// I need to fit n elements in an mxm box where m = 2^y

	idealSquare := math.Sqrt(float64(cidr.AddressCount(network.Net())))
	desiredSquare := math.Pow(2, math.Ceil(math.Log2(idealSquare)))

	rect := image.Rect(0, 0, int(desiredSquare), int(desiredSquare))
	output := image.NewRGBA(rect)
	unallocated := color.RGBA{0, 0, 0, 255}
	inUse := color.RGBA{0, 0, 255, 255}
	reserved := color.RGBA{255, 0, 0, 255}
	allocated := color.RGBA{255, 255, 255, 255}

	info.ForEachIP(func(i int, ip net.IP) {
		x, y := mapPos(i, int(desiredSquare))

		output.Set(x, y, unallocated)
		if info.IPInUse(ip) {
			output.Set(x, y, inUse)
		} else if info.IPReserved(ip) {
			output.Set(x, y, reserved)
		} else if info.IPAllocated(ip) {
			output.Set(x, y, allocated)
		}
	})

	f, err := os.Create("output/" + name + ".png")
	if err != nil {
		log.Fatal(err)
	}

	if err := png.Encode(f, output); err != nil {
		f.Close()
		log.Fatal(err)
	}

	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}

func mapPos(i, dim int) (int, int) {
	var (
		subDim       = dim / 2
		subBlockSize = subDim * subDim
	)

	if dim <= 1 {
		return 0, 0
	}

	x, y := mapPos(i%subBlockSize, subDim)

	if i >= subBlockSize*3 {
		return x + subDim, y + subDim
	}

	if i >= subBlockSize*2 {
		return x, y + subDim
	}

	if i >= subBlockSize {
		return x + subDim, y
	}

	return x, y

}
