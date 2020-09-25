package pass1

/*
=head1 NAME

    export-netspoc - Export data from Netspoc for use in Netspoc-Web

=head1 COPYRIGHT AND DISCLAIMER

    (c) 2020 by Heinz Knutzen <heinz.knutzengmail.com>
    (c) 2014 by Daniel Brunkhorst <daniel.brunkhorstweb.de>

https://github.com/hknutzen/Netspoc-Web

    This program is free software: you can redistribute it &&/|| modify
    it under the terms of the GNU General Public License as published by
    the Free Software Foundation, either version 3 of the License, ||
    (at your option) any later version.
    This program is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY || FITNESS FOR A PARTICULAR PURPOSE. See the
    GNU General Public License for more details.
    You should have received a copy of the GNU General Public License
    along with this program. If !, see <http://www.gnu.org/licenses/>.

=cut
*/

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"github.com/hknutzen/Netspoc/go/pkg/abort"
	"github.com/hknutzen/Netspoc/go/pkg/conf"
	"github.com/hknutzen/Netspoc/go/pkg/diag"
	"github.com/hknutzen/Netspoc/go/pkg/fileop"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func createDirs(dir, path string) {
	path = dir + "/" + path
	err := os.MkdirAll(path, 0777)
	if err != nil {
		abort.Msg("Can't %v", err)
	}
}

func exportJson(dir, path string, data interface{}) {
	path = dir + "/" + path
	fd, err := os.Create(path)
	if err != nil {
		abort.Msg("Can't %v", err)
	}
	enc := json.NewEncoder(fd)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", " ")
	if err := enc.Encode(data); err != nil {
		abort.Msg("%v", err)
	}
	if err := fd.Close(); err != nil {
		abort.Msg("Can't %v", err)
	}
}

func printNetworkIp(n *network, v6 bool) string {
	pIP := n.ip.String()
	var pMask string
	if v6 {
		size, _ := n.mask.Size()
		pMask = strconv.Itoa(size)
	} else {
		pMask = net.IP(n.mask).String()
	}
	return pIP + "/" + pMask
}

type jsonMap map[string]interface{}

// Add attributes ip and nat to dst object.
func ipNatForObject(obj srvObj, dst jsonMap) {
	var ip string
	natMap := make(map[string]string)

	// This code is a modified copy of func address.
	// - It needs to handle objects of type 'Host' instead of 'Subnet'.
	// - Handles dynamic NAT for hosts.
	switch x := obj.(type) {
	case *network:
		getIp := func(n *network) string {
			if n.hidden {
				return "hidden"
			}

			// Don't print mask for loopback network. It needs to have
			// exactly the same address as the corresponding loopback interface.
			if n.loopback {
				return n.ip.String()
			}

			// Print no mask for aggregate with mask 0, for compatibility
			// with old version.
			if n.isAggregate {
				if p, _ := n.mask.Size(); p == 0 {
					return n.ip.String()
				}
			}
			return printNetworkIp(n, n.ipV6)
		}
		ip = getIp(x)
		for tag, natNet := range x.nat {
			natMap[tag] = getIp(natNet)
		}
	case *host:
		getIp := func(h *host, n *network) string {
			if n.dynamic {
				natTag := n.natTag
				if ip, ok := h.nat[natTag]; ok {

					// Single static NAT IP for this host.
					return ip.String()
				}
				if n.hidden {
					return "hidden"
				}

				// Dynamic NAT, take whole network.
				return printNetworkIp(n, n.ipV6)
			}
			if ip := h.ip; ip != nil {
				return mergeIP(ip, n).String()
			}
			r := h.ipRange
			return mergeIP(r[0], n).String() + "-" + mergeIP(r[1], n).String()
		}
		n := x.network
		ip = getIp(x, n)
		for tag, natNet := range n.nat {
			natMap[tag] = getIp(x, natNet)
		}
	case *routerIntf:

		getIp := func(intf *routerIntf, n *network) string {
			if n.dynamic {
				natTag := n.natTag
				if ip, ok := intf.nat[natTag]; ok {

					// Single static NAT IP for this interface.
					return ip.String()
				}
				if n.hidden {
					return "hidden"
				}

				// Dynamic NAT, take whole network.
				return printNetworkIp(n, n.ipV6)
			}
			switch {
			case intf.unnumbered:
				return "unnumbered"
			case intf.short:
				return "short"
			case intf.bridged:
				return "bridged"
			case intf.negotiated:
				// Take whole network.
				return printNetworkIp(n, n.ipV6)
			default:
				return mergeIP(intf.ip, n).String()
			}
		}
		n := x.network
		ip = getIp(x, n)
		for tag, natNet := range n.nat {
			natMap[tag] = getIp(x, natNet)
		}
	}
	dst["ip"] = ip
	if len(natMap) != 0 {
		dst["nat"] = natMap
	}
}

// Zone with network 0/0 doesn't have an aggregate 0/0.
func getZoneName(z *zone) string {
	ip := getZeroIp(z.ipV6)
	mask := getZeroMask(z.ipV6)
	if any := z.ipmask2aggregate[ipmask{string(ip), string(mask)}]; any != nil {
		return any.name
	} else {
		return z.name
	}
}

//#####################################################################
// Setup services
//#####################################################################

// Globally change name of owners from "owner:name" to "name".
func adaptOwnerNames() {
	for _, o := range symTable.owner {
		o.name = strings.TrimPrefix(o.name, "owner:")
	}
}

func ownerForObject(ob srvObj) string {
	if ow := ob.getOwner(); ow != nil {
		return ow.name
	}
	return ""
}

func ownersForObjects(l srvObjList) stringList {
	names := make(map[string]bool)
	for _, ob := range l {
		if name := ownerForObject(ob); name != "" {
			names[name] = true
		}
	}
	result := make(stringList, 0, len(names))
	for name, _ := range names {
		result.push(name)
	}
	sort.Strings(result)
	return result
}

type xOwner map[srvObj][]*owner

func xOwnersForObject(ob srvObj, x xOwner) stringList {
	var result stringList
	for _, ow := range x[ob] {
		result.push(ow.name)
	}
	return result
}

func xOwnersForObjects(l srvObjList, x xOwner) stringList {
	names := make(map[string]bool)
	for _, ob := range l {
		for _, name := range xOwnersForObject(ob, x) {
			names[name] = true
		}
	}
	result := make(stringList, 0, len(names))
	for name, _ := range names {
		result.push(name)
	}
	sort.Strings(result)
	return result
}

func protoDescr(l []*proto) stringList {
	type protoSort struct {
		desc  string
		pType string
		num   int
	}
	var pList []protoSort
	for _, proto0 := range l {
		protocol := proto0
		ptype := protocol.proto
		desc := ptype
		var num int
		switch ptype {
		case "tcp", "udp":
			portCode := func(rangeObj *proto) string {
				v1, v2 := rangeObj.ports[0], rangeObj.ports[1]
				if v1 == v2 {
					return strconv.Itoa(v1)
				} else if v1 == 1 && v2 == 65535 {
					return ""
				} else {
					return strconv.Itoa(v1) + "-" + strconv.Itoa(v2)
				}
			}
			var sport string
			if srcRange := protocol.src; srcRange != nil {
				sport = portCode(srcRange)
			}
			dport := portCode(protocol.dst)
			if sport != "" {
				desc += " " + sport + ":" + dport
			} else if dport != "" {
				desc += " " + dport
				num = protocol.dst.ports[0]
			}
		case "icmp":
			if t := protocol.icmpType; t != -1 {
				s := strconv.Itoa(t)
				if c := protocol.icmpCode; c != -1 {
					desc += " " + s + "/" + strconv.Itoa(c)
				} else {
					desc += " " + s
				}
				num = t
			}
		}
		if m := protocol.modifiers; m != nil {
			if m.dstNet {
				desc += ", dst_net"
			}
			if m.oneway {
				desc += ", oneway"
			}
			if m.reversed {
				desc += ", reversed"
			}
			if m.srcNet {
				desc += ", src_net"
			}
			if m.stateless {
				desc += ", stateless"
			}
		}
		pList = append(pList, protoSort{desc, ptype, num})
	}

	// Sort by protocol, port/type, all (if proto and num are equal)
	sort.Slice(pList, func(i, j int) bool {
		switch strings.Compare(pList[i].pType, pList[j].pType) {
		case -1:
			return true
		case 1:
			return false
		}
		if pList[i].num < pList[j].num {
			return true
		}
		if pList[i].num > pList[j].num {
			return false
		}
		return strings.Compare(pList[i].desc, pList[j].desc) == -1
	})

	result := make(stringList, len(pList))
	for i, x := range pList {
		result[i] = x.desc
	}
	return result
}

func findVisibility(owners, uowners stringList) string {
	var visibility string
	m := make(map[string]bool)
	for _, ow := range owners {
		m[ow] = true
	}
	var DAExtra stringList
	var otherExtra stringList
	for _, ow := range uowners {
		if !m[ow] {
			if strings.HasPrefix(ow, "DA_") {
				DAExtra.push(ow)
			} else {
				otherExtra.push(ow)
			}
		}
	}

	// No known owner or owner of users.
	if len(owners) == 0 && len(uowners) == 0 {
		// Default: private
	} else if len(DAExtra) == 0 && len(otherExtra) == 0 {
		// Set of uowners is subset of owners.
		// Default: private
	} else if len(otherExtra) <= 2 {
		// Restricted visibility
		if len(DAExtra) >= 3 {
			visibility = "DA_"
		}
	} else {
		visibility = "*"
	}
	return visibility
}

// Calculate unique id for set of rules.
// Take first 8 characters of base64 encoded SHA1 hash.
// This gives 8x6=48 bits.
// Collisions would occur with probability of 0.5 for 2^24 different ids.
// We should be extremely safe for up to 2^14 different ids.
func calcRulesKey(rules []jsonMap) string {
	b, err := json.Marshal(rules)
	if err != nil {
		panic(err)
	}
	sum := sha1.Sum(b)
	b = sum[:6]
	digest := base64.StdEncoding.EncodeToString(b)
	result := strings.ReplaceAll(digest, "+", "-")
	result = strings.ReplaceAll(result, "/", "_")
	return result
}

type exportedSvc struct {
	name         string
	description  string
	disableAt    string
	disabled     bool
	user         srvObjList
	objMap       map[srvObj]bool
	jsonRules    []jsonMap
	outerOwners  []string
	outerUowners []string
	owners       []string
	partOwners   []string
	partUowners  []string
	subOwner     string
	uowners      []string
	visible      string
}

// Split service, if 'user' has different values in normalized rules.
func normalizeServicesForExport() []*exportedSvc {
	diag.Progress("Normalize services for export")
	var result []*exportedSvc
	var names stringList
	for n, _ := range services {
		names.push(n)
	}
	sort.Strings(names)
	for _, n := range names {
		s := services[n]
		ipv6 := s.ipV6
		sname := s.name
		ctx := sname
		user := expandGroup(s.user, "user of "+ctx, ipv6, false)
		foreach := s.foreach

		type tmpRule struct {
			objList  srvObjList
			jsonRule jsonMap
		}
		key2rules := make(map[string][]*tmpRule)
		key2user := make(map[string]srvObjList)

		nameList := func(l srvObjList) stringList {
			names := make(stringList, 0, len(l))
			for _, ob := range l {
				names.push(ob.String())
			}
			sort.Strings(names)
			return names
		}
		getUserKey := func(l srvObjList) string {
			return strings.Join(nameList(l), ",")
		}
		seenAsUser := func(l srvObjList) bool {
			_, ok := key2user[getUserKey(l)]
			return ok
		}

		for _, uRule := range s.rules {
			action := uRule.action
			prtList := protoDescr(uRule.prt)
			hasUser := uRule.hasUser

			process := func(elt groupObjList) {
				srcDstListPairs := normalizeSrcDstList(uRule, elt, s)
				for _, srcDstList := range srcDstListPairs {
					srcList, dstList := srcDstList[0], srcDstList[1]

					// Artificially take one of 'src' and 'dst' as user
					// for case like
					// src = user; dst = any:[user];
					listEq := func(l1, l2 srvObjList) bool {
						if len(l1) != len(l2) {
							return false
						}
						for i, el := range l1 {
							if el != l2[i] {
								return false
							}
						}
						return true
					}
					if hasUser == "both" && (!listEq(srcList, dstList) || foreach) {
						if seenAsUser(srcList) {
							hasUser = "src"
						} else if seenAsUser(dstList) {
							hasUser = "dst"
						} else if len(srcList) >= len(dstList) {
							hasUser = "src"
						} else {
							hasUser = "dst"
						}
					}
					rule := &tmpRule{
						jsonRule: jsonMap{
							"action":   action,
							"prt":      prtList,
							"has_user": hasUser,
							"src":      stringList{},
							"dst":      stringList{},
						},
					}
					var userList srvObjList
					if hasUser == "src" {
						rule.objList = dstList
						rule.jsonRule["dst"] = nameList(dstList)
						userList = srcList
					} else {
						if hasUser == "dst" {
							rule.objList = srcList
							rule.jsonRule["src"] = nameList(srcList)
						}
						userList = dstList
					}
					userKey := getUserKey(userList)
					key2rules[userKey] = append(key2rules[userKey], rule)
					key2user[userKey] = userList
				}
			}
			if foreach {
				for _, elt := range user {
					process(groupObjList{elt})
				}
			} else {
				process(user)
			}
		}

		// 'user' has different value for some rules
		// and implicitly we get multiple services with identical name.
		isSplit := len(key2rules) > 1

		// Name of split part is derived from its rules.
		// We might get different split parts with identical rules from
		// auto interfaces. We must re-join these parts to prevent name
		// clashes.
		splitParts := make(map[string]*exportedSvc)

		for userKey, rules := range key2rules {
			userList := key2user[userKey]
			jsonRules := make([]jsonMap, 0, len(rules))
			objMap := make(map[srvObj]bool)
			for _, rule := range rules {
				jsonRules = append(jsonRules, rule.jsonRule)
				for _, ob := range rule.objList {
					objMap[ob] = true
				}
			}
			newName := sname

			// Add extension to make name of split service unique.
			var rulesKey string
			if isSplit {

				// Ignore split part with empty users or only empty rules.
				// This is an relict from expanding auto interfaces.
				if len(userList) == 0 {
					continue
				}
				empty := true
				for i, r := range jsonRules {
					v := r["has_user"].(string)
					if v == "both" || len(rules[i].objList) != 0 {
						empty = false
						break
					}
				}
				if empty {
					continue
				}

				rulesKey = calcRulesKey(jsonRules)
				newName += "(" + rulesKey + ")"

				// Join different split parts having identical rules.
				if other, ok := splitParts[rulesKey]; ok {
					other.user = append(other.user, userList...)
					continue
				}
			}
			newService := &exportedSvc{
				name:        newName,
				description: s.description,
				disableAt:   s.disableAt,
				disabled:    s.disabled,
				user:        userList,
				objMap:      objMap,
				jsonRules:   jsonRules,
			}
			if s.subOwner != nil {
				newService.subOwner = s.subOwner.name
			}
			if rulesKey != "" {
				splitParts[rulesKey] = newService
			}
			result = append(result, newService)
		}
	}
	return result
}

// All objects referenced in rules and in networks and hosts of owners.
var allObjects = make(map[srvObj]bool)

func setupServiceInfo(services []*exportedSvc, pInfo, oInfo xOwner) {
	diag.Progress("Setup service info")

	for _, s := range services {
		users := s.user

		// Non 'user' objects.
		objMap := s.objMap

		// Check, if service contains a coupling rule with only "user" elements.
		isCoupling := false

		for _, rule := range s.jsonRules {
			hasUser := rule["has_user"].(string)
			if hasUser == "both" {
				isCoupling = true
				break
			}
		}
		if isCoupling {
			for _, ob := range users {
				objMap[ob] = true
			}
			users = nil
		}
		var objects srvObjList
		for ob, _ := range objMap {
			objects.push(ob)
		}

		// Store referenced objects for later use during export.
		for _, ob := range append(objects, users...) {
			allObjects[ob] = true
		}

		// Input: owner objects, output: owner names
		owners := ownersForObjects(objects)

		s.owners = owners
		s.partOwners = xOwnersForObjects(objects, pInfo)
		s.outerOwners = xOwnersForObjects(objects, oInfo)

		uowners := ownersForObjects(users)
		s.uowners = uowners
		s.partUowners = xOwnersForObjects(users, pInfo)
		s.outerUowners = xOwnersForObjects(users, oInfo)

		s.visible = findVisibility(owners, uowners)
	}
}

//#####################################################################
// Store part owners for objects which contain objects
// belonging to other owners in pInfo.
//#####################################################################

func setupPartOwners() xOwner {
	diag.Progress("Setup part owners")

	pMap := make(map[srvObj]map[*owner]bool)
	add := func(n *network, ow *owner) {
		oMap := pMap[n]
		if oMap == nil {
			oMap = make(map[*owner]bool)
			pMap[n] = oMap
		}
		oMap[ow] = true
		// debug("%s : %s", n.name, ow.name)
	}

	// Handle hosts of network.
	// Don't handle interfaces here, because
	// - unmanaged interface doesn't have owner and
	// - managed interface isn't part of network.
	for _, n := range symTable.network {
		if n.disabled {
			continue
		}
		netOwner := n.owner
		for _, h := range n.hosts {
			ow := h.owner
			if ow != nil && ow != netOwner {
				add(n, ow)
			}
		}
	}

	// Add owner and partOwner of network to enclosing aggregates and networks.
	for _, n := range symTable.network {
		if n.disabled {
			continue
		}
		var owners []*owner
		for ow, _ := range pMap[n] {
			owners = append(owners, ow)
		}
		if ow := n.owner; ow != nil {
			owners = append(owners, ow)
		}
		up := n.up
		for up != nil {
			for _, ow := range owners {
				if ow != up.owner {
					add(up, ow)
				}
			}
			up = up.up
		}
	}

	// Substitute map by slice.
	pInfo := make(xOwner)
	for ob, m := range pMap {
		s := make([]*owner, 0, len(m))
		for ow, _ := range m {
			s = append(s, ow)
		}
		pInfo[ob] = s
	}
	return pInfo
}

// 1. Store outer owners for hosts, interfaces and networks in oInfo.
//    For network, collect owners from enclosing networks and zone,
//    that are different from networks owner.
//    For host, collect owners of enclosing networks and zone,
//    that are different from hosts owner.
// 2. For each owner, store list of other owners of enclosing objects,
//    that are allowed to watch that owner in eInfo.
//    An outer owner is allowed to select the role of an inner owner,
//    if all assets of the inner owner are located inside of assets
//    that are owned by the outer owner.
// Attribute hideFromOuterOwners is given at inner owner and hides
// from outer owners.
// Attribute showHiddenOwners at outer owner cancels effect of
// hideFromOuterOwners
func setupOuterOwners() (xOwner, map[*owner][]*owner) {
	diag.Progress("Setup outer owners")

	// Find master owner.
	var masterOwner *owner
	for _, ow := range symTable.owner {
		if ow.showAll {
			masterOwner = ow
			diag.Progress("Found master owner: " + ow.name)
			break
		}
	}

	// For each owner, collect intersection of all outer owners.
	owner2outerOwners := make(map[*owner]map[*owner]bool)
	intersectOuterOwners := func(ow *owner, outer []*owner) {
		if m := owner2outerOwners[ow]; m != nil {
			var both []*owner
			for _, ow2 := range outer {
				if m[ow2] {
					both = append(both, ow2)
				}
			}
			outer = both
		}
		m := make(map[*owner]bool)
		for _, ow2 := range outer {
			m[ow2] = true
		}
		owner2outerOwners[ow] = m
	}

	// Create slice from map, sorted by name of owner.
	sortedSlice := func(m map[*owner]bool) []*owner {
		var l []*owner
		for ow, _ := range m {
			l = append(l, ow)
		}
		sort.Slice(l, func(i, j int) bool {
			return l[i].name < l[j].name
		})
		return l
	}

	// Filter owners without attribute showHiddenOwners.
	checkOnlyHidden := func(ow1 *owner, l []*owner) []*owner {
		if ow1 != nil && ow1.hideFromOuterOwners {
			j := 0
			for _, ow2 := range l {
				if ow2.showHiddenOwners {
					l[j] = ow2
					j++
				}
			}
			l = l[:j]
		}
		return l
	}

	// Set outer owners for object and update intersection.
	oInfo := make(xOwner)
	setOuterOwners := func(ob srvObj, ow *owner, outerForObj map[*owner]bool) {
		delete(outerForObj, ow)
		objOuterOwners := sortedSlice(outerForObj)
		objOuterOwners = checkOnlyHidden(ow, objOuterOwners)
		if ow != nil && objOuterOwners != nil {
			intersectOuterOwners(ow, objOuterOwners)
		}
		oInfo[ob] = objOuterOwners
	}

	// Collect outer owners for all objects inside zone.
	for _, z := range zones {
		var zoneOwners []*owner

		// watchingOwners holds list of owners, that have been
		// inherited from areas.
		zoneOwners = z.watchingOwners

		process := func(n *network) {
			outerOwners := make(map[*owner]bool)
			netOwner := n.owner
			up := n
			for {
				up = up.up
				if up == nil {
					break
				}
				outerOwner := up.owner
				if outerOwner == nil {
					continue
				}
				if outerOwner == netOwner {
					continue
				}
				outerOwners[outerOwner] = true
			}
			for _, ow := range zoneOwners {
				outerOwners[ow] = true
			}
			setOuterOwners(n, netOwner, outerOwners)
			if netOwner != nil {
				outerOwners[netOwner] = true
			}
			for _, obj := range n.interfaces {
				ow := obj.owner
				outerForObj := make(map[*owner]bool)
				router := obj.router
				if router.managed != "" || router.routingOnly {
					if masterOwner != nil {
						outerForObj[masterOwner] = true
					}
				} else {
					for ow, _ := range outerOwners {
						outerForObj[ow] = true
					}
				}
				setOuterOwners(obj, ow, outerForObj)
			}
			for _, obj := range n.hosts {
				ow := obj.owner
				outerForObj := make(map[*owner]bool)
				for ow, _ := range outerOwners {
					outerForObj[ow] = true
				}
				setOuterOwners(obj, ow, outerForObj)
			}
		}
		for _, n := range addSubnetworks(z.networks) {
			process(n)
		}
		for _, n := range z.ipmask2aggregate {
			process(n)
		}
	}

	// Intersection of all outer owners of one owner is allowed to take
	// role of corresponding inner owner.
	eInfo := make(map[*owner][]*owner)
	for _, ow := range symTable.owner {
		outerOwners := owner2outerOwners[ow]
		if masterOwner != nil {
			if outerOwners == nil {
				outerOwners = make(map[*owner]bool)
			}
			outerOwners[masterOwner] = true
		}
		eInfo[ow] = sortedSlice(outerOwners)
	}
	return oInfo, eInfo
}

//#####################################################################
// Export NAT-set
// - Relate each network to its owner and part_owners.
// - Build a nat_set for each owner by combining nat_sets of
//   NAT domains of all own networks.
// If owner has exactly one NAT domain, use corresponding nat_set
// to determine IP address of other networks.
// Otherwise multiple nat-sets need to be combined.
// Analyze each network X with multiple NAT tags.
// - If all nat-sets map to the same IP, use this mapping.
// - If some nat-sets map to different IPs, use original IP.
// - If some nat-sets map to the same IP and all other nat-sets
//   map to 'hidden' then ignore hidden in combined nat-set.
// This way, a real NAT tag will not be disabled,
// if it is combined with a hidden NAT tag from same multi-NAT.
//#####################################################################
func exportNatSet(dir string,
	natTag2multinatDef map[string][]natMap, natTag2natType map[string]string,
	pInfo, oInfo xOwner) {

	diag.Progress("Export NAT-sets")
	owner2domains := make(map[string]map[*natDomain]bool)
	for _, n := range symTable.network {
		if n.disabled {
			continue
		}

		// Ignore IPv6 networks where typically no NAT is active.
		if n.ipV6 {
			continue
		}

		d := n.zone.natDomain
		add := func(l stringList) {
			for _, o := range l {
				m := owner2domains[o]
				if m == nil {
					m = make(map[*natDomain]bool)
					owner2domains[o] = m
				}
				m[d] = true
			}
		}
		add(stringList{ownerForObject(n)})
		add(xOwnersForObject(n, pInfo))
		add(xOwnersForObject(n, oInfo))
	}
	for ownerName, _ := range symTable.owner {
		natList := make(stringList, 0)
		if doms := owner2domains[ownerName]; doms != nil {

			// Build union of all natSets of found NAT domains.
			var natSets []natSet
			for d, _ := range doms {
				natSets = append(natSets, d.natSet)
			}
			combined := combineNatSets(natSets, natTag2multinatDef, natTag2natType)
			for tag, _ := range *combined {
				natList.push(tag)
			}
		}
		sort.Strings(natList)

		createDirs(dir, "owner/"+ownerName)
		exportJson(dir, "owner/"+ownerName+"/nat_set", natList)
	}
}

//###################################################################
// Export hosts, networks and zones (represented by aggregate 0/0) for
// each owner and sub_owner.
//###################################################################

// Parameter 'networks' only contains toplevel networks.
// Add subnets recursively.
func addSubnetworks(networks netList) netList {
	var result netList
	for _, n := range networks {
		if subList := n.networks; subList != nil {
			result = append(result, addSubnetworks(subList)...)
		}
	}
	if result != nil {
		return append(result, networks...)
	} else {
		return networks
	}
}

func exportAssets(dir string, pInfo, oInfo xOwner) {
	diag.Progress("Export assets")
	result := make(jsonMap)

	// Returns map with network name(s) as key and list of hosts / interfaces
	// as value.
	exportNetwork := func(net *network, owner string, ownNet bool) jsonMap {
		if net.tunnel {
			return nil
		}
		if net.loopback {
			subResult := make(jsonMap)

			// Show loopback interface as network in assets.
			for _, intf := range net.interfaces {
				allObjects[intf] = true
				subResult[intf.name] = stringList{}
			}
			return subResult
		}
		allObjects[net] = true

		// Export hosts and interfaces.
		var childs srvObjList
		for _, h := range net.hosts {
			childs.push(h)
		}
		for _, i := range net.interfaces {
			childs.push(i)
		}

		// Show only own childs in foreign network.
		if !ownNet {
			netOwner := ownerForObject(net)
			if netOwner != owner {
				j := 0
				for _, ob := range childs {
					if o := ownerForObject(ob); o != "" && o == owner {
						childs[j] = ob
						j++
					}
				}
				childs = childs[:j]
			}
		}

		names := make(stringList, 0)
		for _, ob := range childs {
			allObjects[ob] = true
			names.push(ob.String())
		}
		sort.Strings(names)
		return jsonMap{net.name: names}
	}

	// Different zones can use the same name from ipmask2aggregate '0/0'
	// if they belong to the same zoneCluster.
	// Hence augment existing jsonMap.
	addNetworksInfo := func(owner, name string, add jsonMap) {
		if result[owner] == nil {
			result[owner] = make(jsonMap)
		}
		m1 := result[owner].(jsonMap)
		if m1["anys"] == nil {
			m1["anys"] = make(jsonMap)
		}
		m2 := m1["anys"].(jsonMap)
		if m2[name] == nil {
			m2[name] = make(jsonMap)
		}
		m3 := m2[name].(jsonMap)
		if m3["networks"] == nil {
			m3["networks"] = make(jsonMap)
		}
		m4 := m3["networks"].(jsonMap)
		for k, v := range add {
			m4[k] = v
		}
	}

	for _, z := range zones {

		// All aggregates can be used in rules.
		for _, agg := range z.ipmask2aggregate {
			allObjects[agg] = true
		}

		// Ignore empty zone with only tunnel or unnumbered networks.
		if len(z.networks) == 0 {
			continue
		}

		zoneName := getZoneName(z)
		networks := addSubnetworks(z.networks)

		for _, n := range networks {
			add := func(owner string, ownNet bool) {
				addNetworksInfo(owner, zoneName, exportNetwork(n, owner, ownNet))
			}
			if owner := ownerForObject(n); owner != "" {
				add(owner, true)
			}
			for _, owner := range xOwnersForObject(n, oInfo) {
				add(owner, true)
			}
			for _, owner := range xOwnersForObject(n, pInfo) {
				// Show only own or part_owned networks in foreign zone.
				add(owner, false)
			}
		}
	}

	for owner, _ := range symTable.owner {
		assets := result[owner]
		if assets == nil {
			assets = jsonMap{}
		}
		createDirs(dir, "owner/"+owner)
		exportJson(dir, "owner/"+owner+"/assets", assets)
	}
}

//###################################################################
// Services, rules, users
//###################################################################

// When creating user and service lists for each owner,
// we need to lookup, if an object should be visible by this owner.
func getVisibleOwner(pInfo, oInfo xOwner) map[srvObj]map[string]bool {
	visibleOwner := make(map[srvObj]map[string]bool)
	for ob, _ := range allObjects {
		m := make(map[string]bool)
		visibleOwner[ob] = m
		m[ownerForObject(ob)] = true
		for _, ow := range xOwnersForObject(ob, pInfo) {
			m[ow] = true
		}
		for _, ow := range xOwnersForObject(ob, oInfo) {
			m[ow] = true
		}
	}
	return visibleOwner
}

func exportServices(dir string, list []*exportedSvc) {
	diag.Progress("Export services")
	sInfo := make(jsonMap)
	for _, s := range list {

		// Show artificial owner :unknown if owner is unknown.
		exportedOwners := s.owners
		if len(exportedOwners) == 0 {
			exportedOwners = stringList{":unknown"}
		}
		details := jsonMap{"owner": exportedOwners}
		add := func(key, val string) {
			if val != "" {
				details[key] = val
			}
		}
		add("description", s.description)
		add("sub_owner", s.subOwner)
		add("disable_at", s.disableAt)
		if s.disabled {
			details["disabled"] = 1
		}

		sname := strings.TrimPrefix(s.name, "service:")
		sInfo[sname] = jsonMap{"details": details, "rules": s.jsonRules}
	}
	exportJson(dir, "services", sInfo)
}

func exportUsersAndServiceLists(
	dir string, l []*exportedSvc, pInfo, oInfo xOwner) {

	diag.Progress("Export users and service lists")

	owner2type2sMap := make(map[string]map[string]map[*exportedSvc]bool)
	for _, s := range l {
		addChk := func(l stringList, typ string, chk func(o string) bool) {
			for _, owner := range l {
				if !chk(owner) {
					continue
				}
				if owner2type2sMap[owner] == nil {
					owner2type2sMap[owner] = make(map[string]map[*exportedSvc]bool)
				}
				type2sMap := owner2type2sMap[owner]
				if type2sMap[typ] == nil {
					type2sMap[typ] = make(map[*exportedSvc]bool)
				}
				type2sMap[typ][s] = true
			}
		}
		add := func(l stringList, typ string) {
			addChk(l, typ, func(o string) bool { return true })
		}
		if owner := s.subOwner; owner != "" {
			add(stringList{owner}, "owner")
		}
		add(s.owners, "owner")
		add(s.partOwners, "owner")
		add(s.outerOwners, "owner")
		chkUser := func(owner string) bool {
			return !owner2type2sMap[owner]["owner"][s]
		}
		addChk(s.uowners, "user", chkUser)
		addChk(s.partUowners, "user", chkUser)
		addChk(s.outerUowners, "user", chkUser)
		if visible := s.visible; visible != "" {
			for owner, _ := range symTable.owner {
				type2sMap := owner2type2sMap[owner]
				if type2sMap["owner"][s] {
					continue
				}
				if type2sMap["user"][s] {
					continue
				}
				if visible == "*" || strings.HasPrefix(owner, visible) {
					add(stringList{owner}, "visible")
				}
			}
		}
	}

	visibleOwner := getVisibleOwner(pInfo, oInfo)
	var names stringList
	for name, _ := range symTable.owner {
		names.push(name)
	}
	sort.Strings(names)
	for _, owner := range names {
		type2sMap := owner2type2sMap[owner]
		type2snames := make(map[string]stringList)
		service2users := make(map[string]stringList)
		for _, typ := range []string{"owner", "user", "visible"} {
			sNames := make(stringList, 0)
		SVC:
			for s, _ := range type2sMap[typ] {
				sName := strings.TrimPrefix(s.name, "service:")
				sNames.push(sName)
				var users srvObjList
				switch typ {
				case "visible":
					continue SVC
				case "owner":
					users = s.user
				case "user":
					for _, user := range s.user {
						if visibleOwner[user][owner] {
							users.push(user)
						}
					}
				}

				// Sort result and remove duplicate aggregates from zone
				// clusters.
				uNames := make(stringList, 0, len(users))
				seen := make(map[string]bool)
				for _, user := range users {
					name := user.String()
					if !seen[name] {
						seen[name] = true
						uNames.push(name)
					}
				}
				sort.Strings(uNames)
				service2users[sName] = uNames
			}
			sort.Strings(sNames)
			type2snames[typ] = sNames
		}
		createDirs(dir, "owner/"+owner)
		exportJson(dir, "owner/"+owner+"/service_lists", type2snames)
		exportJson(dir, "owner/"+owner+"/users", service2users)
	}
}

//###################################################################
// Export all objects referenced by rules, users and assets.
//###################################################################

func zoneAndSubnet(obj srvObj, desc jsonMap) {

	// Change loopback interface to equivalent loopback network.
	// Network gets zone attribute added, which is needed in IP search
	// of NetspocWeb.
	if intf, ok := obj.(*routerIntf); ok {
		if intf.loopback {
			obj = intf.network
		}
	}

	n, ok := obj.(*network)
	if !ok {
		return
	}
	z := n.zone
	if c := z.zoneCluster; c != nil {

		// Get deterministic zone for aggregates and networks in zone cluster.
		z = c[0]
	}
	desc["zone"] = getZoneName(z)

	// Netspoc-Web only needs info about subnets in other zone.
	// Attribute name is different for historic reasons.
	if n.hasOtherSubnet {
		desc["is_supernet"] = 1
	}
}

func exportObjects(dir string) {
	diag.Progress("Export objects")
	result := make(jsonMap)
	for obj, _ := range allObjects {
		descr := make(jsonMap)

		// Add key 'ip' and optionally key 'nat'.
		ipNatForObject(obj, descr)

		// Change loopback interface to loopback netwok, but leave name unchanged.
		// Add key 'zone' for network and aggregate.
		// Optionally add key 'is_supernet' for network and aggregate.
		zoneAndSubnet(obj, descr)

		if o := ownerForObject(obj); o != "" {
			descr["owner"] = o
		}
		result[obj.String()] = descr
	}
	exportJson(dir, "objects", result)
}

//###################################################################
// find Email -> Owner
//###################################################################

func exportOwners(outDir string, eInfo map[*owner][]*owner) {
	diag.Progress("Export owners")
	email2owners := make(map[string]map[string]bool)
	for name, ow := range symTable.owner {
		var emails, watchers, eOwners stringList
		add := func(l []string) {
			for _, email := range l {
				oMap := email2owners[email]
				if oMap == nil {
					oMap = make(map[string]bool)
					email2owners[email] = oMap
				}
				oMap[name] = true
			}
		}
		dir := "owner/" + name
		createDirs(outDir, dir)
		add(ow.admins)
		emails = append(emails, ow.admins...)
		add(ow.watchers)
		watchers = append(watchers, ow.watchers...)

		// Handle extending owners.
		for _, eOwner := range eInfo[ow] {

			// Allow both, admins and watchers to look at owner.
			add(eOwner.admins)
			add(eOwner.watchers)
			eOwners.push(eOwner.name)
		}

		export := func(l []string, key, path string) {
			sort.Strings(l)
			out := make([]map[string]string, len(l))
			for i, e := range l {
				m := make(map[string]string)
				m[key] = e
				out[i] = m
			}
			exportJson(outDir, dir+"/"+path, out)
		}
		export(emails, "email", "emails")
		export(watchers, "email", "watchers")
		export(eOwners, "name", "extended_by")
	}

	// Remove owners visible for wildcard addresses '[all]@domain' from
	// all emails 'user@domain' matching that wildcard.
	domain2owners := make(map[string]map[string]bool)
	for email, oMap := range email2owners {
		l := strings.SplitN(email, "@", 2)
		if len(l) == 2 && l[0] == "[all]" {
			domain := l[1]
			domain2owners[domain] = oMap
		}
	}
	for email, oMap := range email2owners {
		l := strings.SplitN(email, "@", 2)
		if len(l) == 2 && l[0] != "[all]" {
			for owner, _ := range domain2owners[l[1]] {
				delete(oMap, owner)
			}
		}
	}

	// Create owner array from owner map.
	email2oList := make(map[string]stringList)
	for e, m := range email2owners {

		// Sort owner names for output.
		l := make(stringList, 0, len(m))
		for o, _ := range m {
			l.push(o)
		}
		sort.Strings(l)
		email2oList[e] = l
	}
	exportJson(outDir, "email", email2oList)
}

func copyPolicyFile(inPath, outDir string) {

	// Copy version information from this file and
	// take modification date for all newly created files.
	// This allows import to RCS even for old versions of netspoc data.
	policyFile := filepath.Join(inPath, "POLICY")
	if fileop.IsRegular(policyFile) {
		cmd := exec.Command(
			"find", outDir, "-type", "f",
			"-exec", "touch", "-r", policyFile, "{}", ";")
		if out, err := cmd.CombinedOutput(); err != nil {
			abort.Msg("executing \"%v\": %v\n%s", cmd, err, out)
		}

		cmd = exec.Command("cp", "-pf", policyFile, outDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			abort.Msg("executing \"%v\": %v\n%s", cmd, err, out)
		}
	}
}

func Export() {
	inDir, outDir := conf.GetArgs()
	ReadNetspoc(inDir)
	MarkDisabled()
	SetZone()
	SetPath()
	natDomains, natTag2natType, multiNAT := DistributeNatInfo()
	FindSubnetsInZone()
	adaptOwnerNames()

	// Copy of services with those services split, that have different 'user'.
	expSvcList := normalizeServicesForExport()
	propagateOwners()
	FindSubnetsInNatDomain(natDomains)
	pInfo := setupPartOwners()
	oInfo, eInfo := setupOuterOwners()
	setupServiceInfo(expSvcList, pInfo, oInfo)

	// Export data
	createDirs(outDir, "")
	exportOwners(outDir, eInfo)
	exportAssets(outDir, pInfo, oInfo)
	exportServices(outDir, expSvcList)
	exportUsersAndServiceLists(outDir, expSvcList, pInfo, oInfo)
	exportObjects(outDir)
	exportNatSet(outDir, multiNAT, natTag2natType, pInfo, oInfo)
	copyPolicyFile(inDir, outDir)
	diag.Progress("Ready")
}
