package patriciaDB

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	DefaultMaxPrefixPerNode         = 4
	DefaultMaxChildrenPerSparseNode = 2
)

type    Prefix      []byte
type	Item        interface{}
type	VisitorFunc func(prefix Prefix, item Item) error


type Trie struct {
	prefix Prefix
	item   Item

//	maxPrefixPerNode         int8
//	maxChildrenPerSparseNode int8

	children childList
}

func NewTrie() *Trie {
	trie := &Trie{}

	/*if trie.maxPrefixPerNode <= 0 {
		trie.maxPrefixPerNode = DefaultMaxPrefixPerNode
	}
	if trie.maxChildrenPerSparseNode <= 0 {
		trie.maxChildrenPerSparseNode = DefaultMaxChildrenPerSparseNode
	}*/
	trie.children = newSparseChildList(DefaultMaxChildrenPerSparseNode)//trie.maxChildrenPerSparseNode)
	return trie
}


// Item returns the item stored in the root of this trie.
func (trie *Trie) Item() Item {
	return trie.item
}

// Insert inserts a new item into the trie using the given prefix. Insert does
// not replace existing items. It returns false if an item was already in place.
func (trie *Trie) Insert(key Prefix, item Item) (inserted bool) {
	return trie.put(key, item, false)
}

// Set works much like Insert, but it always sets the item, possibly replacing
// the item previously inserted.
func (trie *Trie) Set(key Prefix, item Item) {
	trie.put(key, item, true)
}

/*func (trie *Trie) total() int {
	return 1 + trie.children.total()
}*/

// Get returns the item located at key.
//
func (trie *Trie) Get(key Prefix) (item Item) {
	_, node, found, leftover := trie.findSubtree(key)
	if !found || len(leftover) != 0 {
		return nil
	}
	return node.item
}

func (trie *Trie) GetLongestPrefixNode(prefix Prefix) (item Item) {
	var root *Trie 
	var inpPrefix = prefix
	var leftover Prefix
    //trie.dump();
	root = trie
	var prefixlen, lastNonNilPrefix int
	//fmt.Printf("get longest prefixnode")
	for {
		// Compute what part of prefix matches.
	//    fmt.Printf("prefix  = %v root.prefix=%v\n", prefix, root.prefix)
        if(len(prefix) < len(root.prefix)) {
			break
		}
		common := root.longestCommonPrefixLength(prefix)
        prefixlen = prefixlen + common
	    node := trie.Get(inpPrefix[:prefixlen])
        if(node != nil) {
	      lastNonNilPrefix = prefixlen
		}
		prefix = prefix[common:]

		// We used up the whole prefix, subtree found.
		if len(prefix) == 0 {
	//		found = true
			leftover = root.prefix[common:]
			break
		}

		// Partial match means that there is no subtree matching prefix.
		if common < len(root.prefix) {
           prefixlen = prefixlen - common
 			leftover = root.prefix[common:]
			fmt.Printf("leftover = %v\n", leftover)
			break
		}

		// There is some prefix left, move to the children.
		child := root.children.next(prefix[0])
		if child == nil {
			// There is nowhere to continue, there is no subtree matching prefix.
			break
		}

//		parent = root
		root = child
	}
	//fmt.Printf("After for loop, prefixlen = %d\n", prefixlen)
	node := trie.Get(inpPrefix[:lastNonNilPrefix])
    if(node != nil) {
	   return node
	} else {
		return nil
	}
}


// Match returns what Get(prefix) != nil would return. The same warning as for
// Get applies here as well.
func (trie *Trie) Match(prefix Prefix) (matchedExactly bool) {
	return trie.Get(prefix) != nil
}

// Visit calls visitor on every node containing a non-nil item
// in alphabetical order.
//
func (trie *Trie) Visit(visitor VisitorFunc) error {
	return trie.walk(nil, visitor)
}

// Delete deletes the item represented by the given prefix.
//
// True is returned if the matching node was found and deleted.
func (trie *Trie) Delete(key Prefix) (deleted bool) {
	// Nil prefix not allowed.
	if key == nil {
		panic(ErrNilPrefix)
	}

	// Empty trie must be handled explicitly.
	if trie.prefix == nil {
		return false
	}

	// Find the relevant node.
	parent, node, _, leftover := trie.findSubtree(key)
	if len(leftover) != 0 {
		return false
	}

	// If the item is already set to nil, there is nothing to do.
	if node.item == nil {
		return false
	}

	// Delete the item.
	node.item = nil

	// Compact since that might be possible now.
	if compacted := node.compact(); compacted != node {
		if parent == nil {
			*node = *compacted
		} else {
			parent.children.replace(node.prefix[0], compacted)
			*parent = *parent.compact()
		}
	}

	// Remove the node if it has no items.
	if node.empty() {
		// If at the root of the trie, reset
		if parent == nil {
			node.reset()
		} else {
			parent.children.remove(node)
		}
	}

	return true
}

//Internal routines
func (trie *Trie) empty() bool {
	isEmpty := true

	trie.walk(nil, func(prefix Prefix, item Item) error {
		isEmpty = false
		return SkipSubtree
	})

	return isEmpty
}

func (trie *Trie) reset() {
	trie.prefix = nil
	trie.children = newSparseChildList(DefaultMaxPrefixPerNode)//trie.maxPrefixPerNode)
}

func (trie *Trie) put(key Prefix, item Item, replace bool) (inserted bool) {
	// Nil prefix not allowed.
	if key == nil {
		panic(ErrNilPrefix)
	}

	var (
		common int
		node   *Trie = trie
		child  *Trie
	)

	if node.prefix == nil {
		if len(key) <= DefaultMaxPrefixPerNode {//trie.maxPrefixPerNode {
			node.prefix = key
			goto InsertItem
		}
		node.prefix = key[:DefaultMaxPrefixPerNode]//trie.maxPrefixPerNode]
		key = key[DefaultMaxPrefixPerNode:]//trie.maxPrefixPerNode:]
		goto AppendChild
	}

	for {
		// Compute the longest common prefix length.
		common = node.longestCommonPrefixLength(key)
		key = key[common:]

		// Only a part matches, split.
		if common < len(node.prefix) {
			goto SplitPrefix
		}

		// common == len(node.prefix) since never (common > len(node.prefix))
		// common == len(former key) <-> 0 == len(key)
		// -> former key == node.prefix
		if len(key) == 0 {
			goto InsertItem
		}

		// Check children for matching prefix.
		child = node.children.next(key[0])
		if child == nil {
			goto AppendChild
		}
		node = child
	}

SplitPrefix:
	// Split the prefix if necessary.
	child = new(Trie)
	*child = *node
	*node = *NewTrie()
	node.prefix = child.prefix[:common]
	child.prefix = child.prefix[common:]
	child = child.compact()
	node.children = node.children.add(child)

AppendChild:
	// Keep appending children until whole prefix is inserted.
	// This loop starts with empty node.prefix that needs to be filled.
	for len(key) != 0 {
		child := NewTrie()
		if len(key) <= DefaultMaxPrefixPerNode{//trie.maxPrefixPerNode {
			child.prefix = key
			node.children = node.children.add(child)
			node = child
			goto InsertItem
		} else {
			child.prefix = key[:DefaultMaxPrefixPerNode]//:trie.maxPrefixPerNode]
			key = key[DefaultMaxPrefixPerNode:]//trie.maxPrefixPerNode:]
			node.children = node.children.add(child)
			node = child
		}
	}

InsertItem:
	// Try to insert the item if possible.
	if replace || node.item == nil {
		node.item = item
		return true
	}
	return false
}

func (trie *Trie) compact() *Trie {
	// Only a node with a single child can be compacted.
	if trie.children.length() != 1 {
		return trie
	}

	child := trie.children.head()

	// If any item is set, we cannot compact since we want to retain
	// the ability to do searching by key. This makes compaction less usable,
	// but that simply cannot be avoided.
	if trie.item != nil || child.item != nil {
		return trie
	}

	// Make sure the combined prefixes fit into a single node.
	if len(trie.prefix)+len(child.prefix) > DefaultMaxPrefixPerNode {//trie.maxPrefixPerNode {
		return trie
	}

	// Concatenate the prefixes, move the items.
	child.prefix = append(trie.prefix, child.prefix...)
	if trie.item != nil {
		child.item = trie.item
	}

	return child
}

func (trie *Trie) findSubtree(prefix Prefix) (parent *Trie, root *Trie, found bool, leftover Prefix) {
	// Find the subtree matching prefix.
	root = trie
	for {
		// Compute what part of prefix matches.
		common := root.longestCommonPrefixLength(prefix)
		prefix = prefix[common:]

		// We used up the whole prefix, subtree found.
		if len(prefix) == 0 {
			found = true
			leftover = root.prefix[common:]
			return
		}

		// Partial match means that there is no subtree matching prefix.
		if common < len(root.prefix) {
			leftover = root.prefix[common:]
			return
		}

		// There is some prefix left, move to the children.
		child := root.children.next(prefix[0])
		if child == nil {
			// There is nowhere to continue, there is no subtree matching prefix.
			return
		}

		parent = root
		root = child
	}
}

func (trie *Trie) walk(actualRootPrefix Prefix, visitor VisitorFunc) error {
	var prefix Prefix
	// Allocate a bit more space for prefix at the beginning.
	if actualRootPrefix == nil {
		prefix = make(Prefix, 32+len(trie.prefix))
		copy(prefix, trie.prefix)
		prefix = prefix[:len(trie.prefix)]
	} else {
		prefix = make(Prefix, 32+len(actualRootPrefix))
		copy(prefix, actualRootPrefix)
		prefix = prefix[:len(actualRootPrefix)]
	}

	// Visit the root first. Not that this works for empty trie as well since
	// in that case item == nil && len(children) == 0.
	if trie.item != nil {
		if err := visitor(prefix, trie.item); err != nil {
			if err == SkipSubtree {
				return nil
			}
			return err
		}
	}

	// Then continue to the children.
	return trie.children.walk(&prefix, visitor)
}

func (trie *Trie) longestCommonPrefixLength(prefix Prefix) (i int) {
//	fmt.Printf("len(prefix)=%d len(trie.prefix)=%d\n", len(prefix), len(trie.prefix))
	for ; i < len(prefix) && i < len(trie.prefix) && prefix[i] == trie.prefix[i]; i++ {
	}
	return
}

func (trie *Trie) dump() string {
	writer := &bytes.Buffer{}
	trie.print(writer, 0)
	return writer.String()
}

func (trie *Trie) print(writer io.Writer, indent int) {
	//fmt.Fprintf(writer, "%s%s %v\n", strings.Repeat(" ", indent), string(trie.prefix), trie.item)
	fmt.Printf("%s%s %v\n", strings.Repeat(" ", indent), string(trie.prefix), trie.item)
	//trie.children.print(writer, indent+2)
}

// Errors ----------------------------------------------------------------------

var (
	SkipSubtree  = errors.New("Skip this subtree")
	ErrNilPrefix = errors.New("Nil prefix passed into a method call")
)