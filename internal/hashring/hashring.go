package hashring

import (
	"hash/crc32"
	"sort"
	"strconv"
)

type HashRing struct {
	nodes         []Node
	vnodesPerNode int
}

type Node struct {
	hash   int
	server string
}

func NewHashRing(vnodesPerNode int) *HashRing {
	return &HashRing{
		vnodesPerNode: vnodesPerNode,
	}
}

func (h *HashRing) AddServer(server string) {
	for i := 0; i < h.vnodesPerNode; i++ {
		vnode := server + "#" + strconv.Itoa(i)
		hash := int(crc32.ChecksumIEEE([]byte(vnode)))
		h.nodes = append(h.nodes, Node{hash: hash, server: server})
	}
	sort.Slice(h.nodes, func(i, j int) bool {
		return h.nodes[i].hash < h.nodes[j].hash
	})
}

func (h *HashRing) RemoveServer(server string) {
	var newNodes []Node
	for _, node := range h.nodes {
		if node.server != server {
			newNodes = append(newNodes, node)
		}
	}
	h.nodes = newNodes
}

func (h *HashRing) GetServer(key string) string {
	if len(h.nodes) == 0 {
		return ""
	}
	hash := int(crc32.ChecksumIEEE([]byte(key)))
	idx := sort.Search(len(h.nodes), func(i int) bool {
		return h.nodes[i].hash >= hash
	})
	if idx == len(h.nodes) {
		idx = 0
	}
	return h.nodes[idx].server
}
