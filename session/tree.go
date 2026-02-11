package session

import (
	"fmt"
	"sync"
	"time"
)

// SessionNode represents a node in the session tree
type SessionNode struct {
	ID         string      `json:"id"`
	Session    *Session    `json:"session"`
	ParentID   string      `json:"parent_id,omitempty"`
	ChildIDs   []string    `json:"child_ids,omitempty"`
	BranchInfo *BranchInfo `json:"branch_info,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
}

// BranchInfo contains metadata about a branch
type BranchInfo struct {
	Name        string    `json:"name"`        // Human-readable branch name
	Description string    `json:"description"` // Branch description
	IsMain      bool      `json:"is_main"`     // Whether this is the main branch
	IsMerged    bool      `json:"is_merged"`   // Whether this branch was merged back
	MergedFrom  []string  `json:"merged_from"` // List of branch IDs that were merged
	CreatedAt   time.Time `json:"created_at"`
	CreatedBy   string    `json:"created_by"` // What created this branch (user, system, auto)
}

// SessionTree manages a tree structure of sessions with branching
type SessionTree struct {
	nodes    map[string]*SessionNode
	rootID   string
	mu       sync.RWMutex
	maxDepth int
}

// NewSessionTree creates a new session tree
func NewSessionTree(rootSession *Session) (*SessionTree, error) {
	if rootSession == nil {
		return nil, fmt.Errorf("root session cannot be nil")
	}

	tree := &SessionTree{
		nodes:    make(map[string]*SessionNode),
		maxDepth: 10, // Default maximum depth
	}

	// Create root node
	rootID := rootSession.Key
	if rootID == "" {
		rootID = "root"
	}

	rootNode := &SessionNode{
		ID:       rootID,
		Session:  rootSession,
		ChildIDs: []string{},
		BranchInfo: &BranchInfo{
			Name:      "main",
			IsMain:    true,
			CreatedAt: time.Now(),
			CreatedBy: "system",
		},
		CreatedAt: time.Now(),
	}

	tree.nodes[rootID] = rootNode
	tree.rootID = rootID

	return tree, nil
}

// CreateBranch creates a new branch from the current session state
func (t *SessionTree) CreateBranch(parentID string, branchSession *Session, branchName string, createdBy string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Verify parent exists
	parent, ok := t.nodes[parentID]
	if !ok {
		return "", fmt.Errorf("parent node %s not found", parentID)
	}

	// Check depth limit
	depth := t.calculateDepth(parentID)
	if depth >= t.maxDepth {
		return "", fmt.Errorf("maximum branch depth %d exceeded", t.maxDepth)
	}

	// Create branch session (copy of parent with new messages)
	if branchSession == nil {
		branchSession = &Session{
			Key:       fmt.Sprintf("%s-branch-%d", parentID, len(parent.ChildIDs)+1),
			Messages:  make([]Message, len(parent.Session.Messages)),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Metadata:  make(map[string]interface{}),
		}
		copy(branchSession.Messages, parent.Session.Messages)
	}

	// Create branch node
	branchID := branchSession.Key
	branchNode := &SessionNode{
		ID:       branchID,
		Session:  branchSession,
		ParentID: parentID,
		ChildIDs: []string{},
		BranchInfo: &BranchInfo{
			Name:      branchName,
			IsMain:    false,
			CreatedAt: time.Now(),
			CreatedBy: createdBy,
		},
		CreatedAt: time.Now(),
	}

	// Add to tree
	t.nodes[branchID] = branchNode
	parent.ChildIDs = append(parent.ChildIDs, branchID)

	return branchID, nil
}

// GetNode retrieves a node by ID
func (t *SessionTree) GetNode(id string) (*SessionNode, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	node, ok := t.nodes[id]
	if !ok {
		return nil, fmt.Errorf("node %s not found", id)
	}

	return node, nil
}

// GetRoot returns the root node
func (t *SessionTree) GetRoot() (*SessionNode, error) {
	return t.GetNode(t.rootID)
}

// GetPath returns the path from root to the specified node
func (t *SessionTree) GetPath(id string) ([]*SessionNode, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	path := []*SessionNode{}
	currentID := id

	for currentID != "" {
		node, ok := t.nodes[currentID]
		if !ok {
			return nil, fmt.Errorf("node %s not found", currentID)
		}

		path = append([]*SessionNode{node}, path...)
		currentID = node.ParentID
	}

	return path, nil
}

// GetChildren returns all children of a node
func (t *SessionTree) GetChildren(id string) ([]*SessionNode, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	node, ok := t.nodes[id]
	if !ok {
		return nil, fmt.Errorf("node %s not found", id)
	}

	children := make([]*SessionNode, len(node.ChildIDs))
	for i, childID := range node.ChildIDs {
		child, ok := t.nodes[childID]
		if !ok {
			continue
		}
		children[i] = child
	}

	return children, nil
}

// GetBranches returns all branches (non-main nodes)
func (t *SessionTree) GetBranches() []*SessionNode {
	t.mu.RLock()
	defer t.mu.RUnlock()

	branches := []*SessionNode{}
	for _, node := range t.nodes {
		if node.BranchInfo != nil && !node.BranchInfo.IsMain {
			branches = append(branches, node)
		}
	}

	return branches
}

// MergeBranch merges a branch back into its parent
func (t *SessionTree) MergeBranch(branchID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	branch, ok := t.nodes[branchID]
	if !ok {
		return fmt.Errorf("branch %s not found", branchID)
	}

	if branch.BranchInfo == nil || branch.BranchInfo.IsMain {
		return fmt.Errorf("cannot merge main branch")
	}

	parent, ok := t.nodes[branch.ParentID]
	if !ok {
		return fmt.Errorf("parent node %s not found", branch.ParentID)
	}

	// Append branch messages to parent
	parent.Session.mu.Lock()
	parent.Session.Messages = append(parent.Session.Messages, branch.Session.Messages...)
	parent.Session.UpdatedAt = time.Now()
	parent.Session.mu.Unlock()

	// Mark branch as merged
	branch.BranchInfo.IsMerged = true
	if parent.BranchInfo == nil {
		parent.BranchInfo = &BranchInfo{}
	}
	parent.BranchInfo.MergedFrom = append(parent.BranchInfo.MergedFrom, branchID)

	return nil
}

// DeleteNode removes a node and its subtree
func (t *SessionTree) DeleteNode(id string, recursive bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	node, ok := t.nodes[id]
	if !ok {
		return fmt.Errorf("node %s not found", id)
	}

	if id == t.rootID {
		return fmt.Errorf("cannot delete root node")
	}

	// Collect all nodes to delete
	nodesToDelete := []string{id}
	if recursive {
		nodesToDelete = t.collectDescendants(id, nodesToDelete)
	}

	// Remove from parent's child list
	parent, ok := t.nodes[node.ParentID]
	if ok {
		newChildIDs := []string{}
		for _, childID := range parent.ChildIDs {
			if childID != id {
				newChildIDs = append(newChildIDs, childID)
			}
		}
		parent.ChildIDs = newChildIDs
	}

	// Delete nodes
	for _, nodeID := range nodesToDelete {
		delete(t.nodes, nodeID)
	}

	return nil
}

// collectDescendants collects all descendant IDs of a node
func (t *SessionTree) collectDescendants(id string, collected []string) []string {
	node, ok := t.nodes[id]
	if !ok {
		return collected
	}

	for _, childID := range node.ChildIDs {
		collected = append(collected, childID)
		collected = t.collectDescendants(childID, collected)
	}

	return collected
}

// calculateDepth calculates the depth of a node
func (t *SessionTree) calculateDepth(id string) int {
	depth := 0
	currentID := id

	for currentID != "" {
		node, ok := t.nodes[currentID]
		if !ok {
			break
		}
		currentID = node.ParentID
		depth++
	}

	return depth
}

// SetMaxDepth sets the maximum branch depth
func (t *SessionTree) SetMaxDepth(depth int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.maxDepth = depth
}

// GetMaxDepth returns the maximum branch depth
func (t *SessionTree) GetMaxDepth() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.maxDepth
}

// ListNodes returns all nodes in the tree
func (t *SessionTree) ListNodes() []*SessionNode {
	t.mu.RLock()
	defer t.mu.RUnlock()

	nodes := make([]*SessionNode, 0, len(t.nodes))
	for _, node := range t.nodes {
		nodes = append(nodes, node)
	}

	return nodes
}

// CountNodes returns the total number of nodes
func (t *SessionTree) CountNodes() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return len(t.nodes)
}

// FindNodesByBranchName finds nodes with matching branch name
func (t *SessionTree) FindNodesByBranchName(name string) []*SessionNode {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var matching []*SessionNode
	for _, node := range t.nodes {
		if node.BranchInfo != nil && node.BranchInfo.Name == name {
			matching = append(matching, node)
		}
	}

	return matching
}

// GetStatistics returns statistics about the session tree
func (t *SessionTree) GetStatistics() *TreeStatistics {
	t.mu.RLock()
	defer t.mu.RUnlock()

	stats := &TreeStatistics{
		TotalNodes:  len(t.nodes),
		MaxDepth:    t.maxDepth,
		ActualDepth: 0,
		BranchCount: 0,
		MergedCount: 0,
	}

	// Calculate actual depth and counts
	for _, node := range t.nodes {
		depth := t.calculateDepth(node.ID)
		if depth > stats.ActualDepth {
			stats.ActualDepth = depth
		}

		if node.BranchInfo != nil {
			if !node.BranchInfo.IsMain {
				stats.BranchCount++
			}
			if node.BranchInfo.IsMerged {
				stats.MergedCount++
			}
		}
	}

	return stats
}

// TreeStatistics contains statistics about the session tree
type TreeStatistics struct {
	TotalNodes  int `json:"total_nodes"`
	MaxDepth    int `json:"max_depth"`
	ActualDepth int `json:"actual_depth"`
	BranchCount int `json:"branch_count"`
	MergedCount int `json:"merged_count"`
}

// SwitchBranch switches the active branch to a different branch
func (t *SessionTree) SwitchBranch(fromID, toID string) (*Session, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Verify both nodes exist
	_, ok := t.nodes[fromID]
	if !ok {
		return nil, fmt.Errorf("source node %s not found", fromID)
	}

	toNode, ok := t.nodes[toID]
	if !ok {
		return nil, fmt.Errorf("target node %s not found", toID)
	}

	// Find common ancestor
	commonAncestor := t.findCommonAncestor(fromID, toID)
	if commonAncestor == "" {
		return nil, fmt.Errorf("nodes %s and %s are not related", fromID, toID)
	}

	// Return the target session
	return toNode.Session, nil
}

// findCommonAncestor finds the common ancestor of two nodes
func (t *SessionTree) findCommonAncestor(id1, id2 string) string {
	ancestors1 := make(map[string]bool)
	currentID := id1

	for currentID != "" {
		ancestors1[currentID] = true
		node, ok := t.nodes[currentID]
		if !ok {
			break
		}
		currentID = node.ParentID
	}

	currentID = id2
	for currentID != "" {
		if ancestors1[currentID] {
			return currentID
		}
		node, ok := t.nodes[currentID]
		if !ok {
			break
		}
		currentID = node.ParentID
	}

	return ""
}

// CompareSessions compares two sessions and returns differences
func (t *SessionTree) CompareSessions(id1, id2 string) (*SessionDiff, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	node1, ok := t.nodes[id1]
	if !ok {
		return nil, fmt.Errorf("node %s not found", id1)
	}

	node2, ok := t.nodes[id2]
	if !ok {
		return nil, fmt.Errorf("node %s not found", id2)
	}

	diff := &SessionDiff{
		ID1:       id1,
		ID2:       id2,
		Messages1: len(node1.Session.Messages),
		Messages2: len(node2.Session.Messages),
	}

	// Compare message counts
	if len(node1.Session.Messages) < len(node2.Session.Messages) {
		diff.AddedMessages = len(node2.Session.Messages) - len(node1.Session.Messages)
		diff.AddedContent = node2.Session.Messages[len(node1.Session.Messages):]
	} else if len(node1.Session.Messages) > len(node2.Session.Messages) {
		diff.RemovedMessages = len(node1.Session.Messages) - len(node2.Session.Messages)
		diff.RemovedContent = node1.Session.Messages[len(node2.Session.Messages):]
	}

	return diff, nil
}

// SessionDiff represents differences between two sessions
type SessionDiff struct {
	ID1             string    `json:"id1"`
	ID2             string    `json:"id2"`
	Messages1       int       `json:"messages1"`
	Messages2       int       `json:"messages2"`
	AddedMessages   int       `json:"added_messages"`
	RemovedMessages int       `json:"removed_messages"`
	AddedContent    []Message `json:"added_content,omitempty"`
	RemovedContent  []Message `json:"removed_content,omitempty"`
}
