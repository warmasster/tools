package main

import (
	"testing"
	"time"
)

// Un nodo que no cuelga de la raíz y apunta a un ciclo (como el registro 0 de la
// MFT, que era su propio padre) no debe dejar colgados alive/pathOf/addToAncestors.
func TestTreeWalksStopOnCycles(t *testing.T) {
	tr := newTree(`C:\`, 8)
	for i := 0; i < 8; i++ {
		tr.Nodes = append(tr.Nodes, Node{Parent: none, First: none, Next: none})
	}
	tr.Root = 5
	tr.Nodes[0].Parent = 0 // se apunta a sí mismo
	tr.Nodes[7].Parent = 0
	tr.Nodes[3].Parent, tr.Nodes[4].Parent = 4, 3 // ciclo A↔B

	done := make(chan bool, 1)
	go func() {
		ok := tr.alive(7) || tr.alive(3)
		tr.pathOf(7)
		tr.relPath(3)
		tr.addToAncestors(7, 1, 1, 1)
		done <- ok
	}()
	select {
	case ok := <-done:
		if ok {
			t.Error("un nodo que no llega a la raíz no está vivo")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("los recorridos del árbol se han quedado en bucle")
	}
}
