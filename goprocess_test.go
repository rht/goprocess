package goprocess

import (
	"syscall"
	"testing"
	"time"
)

type tree struct {
	Process
	c []tree
}

func setupHierarchy(p Process) tree {
	t := func(n Process, ts ...tree) tree {
		return tree{n, ts}
	}

	a := WithParent(p)
	b1 := WithParent(a)
	b2 := WithParent(a)
	c1 := WithParent(b1)
	c2 := WithParent(b1)
	c3 := WithParent(b2)
	c4 := WithParent(b2)

	return t(a, t(b1, t(c1), t(c2)), t(b2, t(c3), t(c4)))
}

func TestClosingClosed(t *testing.T) {

	a := WithParent(Background())
	b := WithParent(a)

	Q := make(chan string, 3)

	go func() {
		<-a.Closing()
		Q <- "closing"
		b.Close()
	}()

	go func() {
		<-a.Closed()
		Q <- "closed"
	}()

	go func() {
		a.Close()
		Q <- "closed"
	}()

	if q := <-Q; q != "closing" {
		t.Error("order incorrect. closing not first")
	}
	if q := <-Q; q != "closed" {
		t.Error("order incorrect. closing not first")
	}
	if q := <-Q; q != "closed" {
		t.Error("order incorrect. closing not first")
	}
}

func TestChildFunc(t *testing.T) {
	a := WithParent(Background())

	wait1 := make(chan struct{})
	wait2 := make(chan struct{})
	wait3 := make(chan struct{})
	wait4 := make(chan struct{})
	go func() {
		a.Close()
		wait4 <- struct{}{}
	}()

	a.Go(func(process Process) {
		wait1 <- struct{}{}
		<-wait2
		wait3 <- struct{}{}
	})

	<-wait1
	select {
	case <-wait3:
		t.Error("should not be closed yet")
	case <-wait4:
		t.Error("should not be closed yet")
	case <-a.Closed():
		t.Error("should not be closed yet")
	default:
	}

	wait2 <- struct{}{}

	select {
	case <-wait3:
	case <-time.After(time.Second):
		t.Error("should be closed now")
	}

	select {
	case <-wait4:
	case <-time.After(time.Second):
		t.Error("should be closed now")
	}
}

func TestTeardownCalledOnce(t *testing.T) {
	a := setupHierarchy(Background())

	onlyOnce := func() func() error {
		count := 0
		return func() error {
			count++
			if count > 1 {
				t.Error("called", count, "times")
			}
			return nil
		}
	}

	setTeardown := func(t tree, tf TeardownFunc) {
		t.Process.(*process).teardown = tf
	}

	setTeardown(a, onlyOnce())
	setTeardown(a.c[0], onlyOnce())
	setTeardown(a.c[0].c[0], onlyOnce())
	setTeardown(a.c[0].c[1], onlyOnce())
	setTeardown(a.c[1], onlyOnce())
	setTeardown(a.c[1].c[0], onlyOnce())
	setTeardown(a.c[1].c[1], onlyOnce())

	a.c[0].c[0].Close()
	a.c[0].c[0].Close()
	a.c[0].c[0].Close()
	a.c[0].c[0].Close()
	a.c[0].Close()
	a.c[0].Close()
	a.c[0].Close()
	a.c[0].Close()
	a.Close()
	a.Close()
	a.Close()
	a.Close()
	a.c[1].Close()
	a.c[1].Close()
	a.c[1].Close()
	a.c[1].Close()
}

func TestOnClosed(t *testing.T) {

	Q := make(chan string, 10)
	p := WithParent(Background())
	a := setupHierarchy(p)

	go onClosedStr(Q, "0", a.c[0])
	go onClosedStr(Q, "10", a.c[1].c[0])
	go onClosedStr(Q, "", a)
	go onClosedStr(Q, "00", a.c[0].c[0])
	go onClosedStr(Q, "1", a.c[1])
	go onClosedStr(Q, "01", a.c[0].c[1])
	go onClosedStr(Q, "11", a.c[1].c[1])

	go p.Close()

	testStrs(t, Q, "00", "01", "10", "11")
	testStrs(t, Q, "00", "01", "10", "11")
	testStrs(t, Q, "00", "01", "10", "11")
	testStrs(t, Q, "00", "01", "10", "11")
	testStrs(t, Q, "0", "1")
	testStrs(t, Q, "0", "1")
	testStrs(t, Q, "")
}

func TestWaitFor(t *testing.T) {

	Q := make(chan string, 5)
	a := WithParent(Background())
	b := WithParent(Background())
	c := WithParent(Background())
	d := WithParent(Background())
	e := WithParent(Background())

	go onClosedStr(Q, "a", a)
	go onClosedStr(Q, "b", b)
	go onClosedStr(Q, "c", c)
	go onClosedStr(Q, "d", d)
	go onClosedStr(Q, "e", e)

	testNone(t, Q)
	a.WaitFor(b)
	a.WaitFor(c)
	b.WaitFor(d)
	e.WaitFor(d)
	testNone(t, Q)

	go a.Close() // should do nothing.
	testNone(t, Q)

	go e.Close()
	testNone(t, Q)

	d.Close()
	testStrs(t, Q, "d", "e")
	testStrs(t, Q, "d", "e")

	c.Close()
	testStrs(t, Q, "c")

	b.Close()
	testStrs(t, Q, "a", "b")
	testStrs(t, Q, "a", "b")
}

func TestAddChildNoWait(t *testing.T) {

	Q := make(chan string, 5)
	a := WithParent(Background())
	b := WithParent(Background())
	c := WithParent(Background())
	d := WithParent(Background())
	e := WithParent(Background())

	go onClosedStr(Q, "a", a)
	go onClosedStr(Q, "b", b)
	go onClosedStr(Q, "c", c)
	go onClosedStr(Q, "d", d)
	go onClosedStr(Q, "e", e)

	testNone(t, Q)
	a.AddChildNoWait(b)
	a.AddChildNoWait(c)
	b.AddChildNoWait(d)
	e.AddChildNoWait(d)
	testNone(t, Q)

	b.Close()
	testStrs(t, Q, "b", "d")
	testStrs(t, Q, "b", "d")

	a.Close()
	testStrs(t, Q, "a", "c")
	testStrs(t, Q, "a", "c")

	e.Close()
	testStrs(t, Q, "e")
}

func TestAddChild(t *testing.T) {

	a := WithParent(Background())
	b := WithParent(Background())
	c := WithParent(Background())
	d := WithParent(Background())
	e := WithParent(Background())
	Q := make(chan string, 5)

	go onClosedStr(Q, "a", a)
	go onClosedStr(Q, "b", b)
	go onClosedStr(Q, "c", c)
	go onClosedStr(Q, "d", d)
	go onClosedStr(Q, "e", e)

	testNone(t, Q)
	a.AddChild(b)
	a.AddChild(c)
	b.AddChild(d)
	e.AddChild(d)
	testNone(t, Q)

	go b.Close()
	testNone(t, Q)
	d.Close()
	testStrs(t, Q, "b", "d")
	testStrs(t, Q, "b", "d")

	go a.Close()
	testNone(t, Q)
	c.Close()
	testStrs(t, Q, "a", "c")
	testStrs(t, Q, "a", "c")

	e.Close()
	testStrs(t, Q, "e")
}

func TestGoChildrenClose(t *testing.T) {

	var a, b, c, d, e Process
	var ready = make(chan struct{})
	var bWait = make(chan struct{})
	var cWait = make(chan struct{})
	var dWait = make(chan struct{})
	var eWait = make(chan struct{})

	a = WithParent(Background())
	a.Go(func(p Process) {
		b = p
		b.Go(func(p Process) {
			c = p
			ready <- struct{}{}
			<-cWait
		})
		ready <- struct{}{}
		<-bWait
	})
	a.Go(func(p Process) {
		d = p
		d.Go(func(p Process) {
			e = p
			ready <- struct{}{}
			<-eWait
		})
		ready <- struct{}{}
		<-dWait
	})

	<-ready
	<-ready
	<-ready
	<-ready

	Q := make(chan string, 5)

	go onClosedStr(Q, "a", a)
	go onClosedStr(Q, "b", b)
	go onClosedStr(Q, "c", c)
	go onClosedStr(Q, "d", d)
	go onClosedStr(Q, "e", e)

	testNone(t, Q)
	go a.Close()
	testNone(t, Q)

	bWait <- struct{}{} // relase b
	go b.Close()
	testNone(t, Q)

	cWait <- struct{}{} // relase c
	<-c.Closed()
	<-b.Closed()
	testStrs(t, Q, "b", "c")
	testStrs(t, Q, "b", "c")

	eWait <- struct{}{} // release e
	<-e.Closed()
	testStrs(t, Q, "e")

	dWait <- struct{}{} // releasse d
	<-d.Closed()
	<-a.Closed()
	testStrs(t, Q, "a", "d")
	testStrs(t, Q, "a", "d")
}

func TestCloseAfterChildren(t *testing.T) {

	var a, b, c, d, e Process

	var ready = make(chan struct{})

	a = WithParent(Background())
	a.Go(func(p Process) {
		b = p
		b.Go(func(p Process) {
			c = p
			ready <- struct{}{}
			<-p.Closing() // wait till we're told to close (parents mustnt)
		})
		ready <- struct{}{}
	})
	a.Go(func(p Process) {
		d = p
		d.Go(func(p Process) {
			e = p
			ready <- struct{}{}
			<-p.Closing() // wait till we're told to close (parents mustnt)
		})
		ready <- struct{}{}
	})

	<-ready
	<-ready
	<-ready
	<-ready

	Q := make(chan string, 5)

	go onClosedStr(Q, "a", a)
	go onClosedStr(Q, "b", b)
	go onClosedStr(Q, "c", c)
	go onClosedStr(Q, "d", d)
	go onClosedStr(Q, "e", e)

	aDone := make(chan struct{})
	bDone := make(chan struct{})

	testNone(t, Q)
	go func() {
		a.CloseAfterChildren()
		aDone <- struct{}{}
	}()
	testNone(t, Q)

	go func() {
		b.CloseAfterChildren()
		bDone <- struct{}{}
	}()
	testNone(t, Q)

	c.Close()
	<-bDone
	<-b.Closed()
	testStrs(t, Q, "b", "c")
	testStrs(t, Q, "b", "c")

	e.Close()
	testStrs(t, Q, "e")

	d.Close()
	<-aDone
	<-a.Closed()
	testStrs(t, Q, "a", "d")
	testStrs(t, Q, "a", "d")
}

func TestBackground(t *testing.T) {
	// test it hangs indefinitely:
	b := Background()
	go b.Close()

	select {
	case <-b.Closing():
		t.Error("b.Closing() closed :(")
	default:
	}
}

func TestWithSignals(t *testing.T) {
	p := WithSignals(syscall.SIGABRT)
	testNotClosed(t, p)

	syscall.Kill(syscall.Getpid(), syscall.SIGABRT)
	testClosed(t, p)
}

func testClosing(t *testing.T, p Process) {
	select {
	case <-p.Closing():
	case <-time.After(50 * time.Millisecond):
		t.Fatal("should be closing")
	}
}

func testNotClosing(t *testing.T, p Process) {
	select {
	case <-p.Closing():
		t.Fatal("should not be closing")
	case <-p.Closed():
		t.Fatal("should not be closed")
	default:
	}
}

func testClosed(t *testing.T, p Process) {
	select {
	case <-p.Closed():
	case <-time.After(50 * time.Millisecond):
		t.Fatal("should be closed")
	}
}

func testNotClosed(t *testing.T, p Process) {
	select {
	case <-p.Closed():
		t.Fatal("should not be closed")
	case <-time.After(50 * time.Millisecond):
	}
}

func testNone(t *testing.T, c <-chan string) {
	select {
	case <-c:
		t.Fatal("none should be closed")
	default:
	}
}

func testStrs(t *testing.T, Q <-chan string, ss ...string) {
	s1 := <-Q
	for _, s2 := range ss {
		if s1 == s2 {
			return
		}
	}
	t.Error("context not in group:", s1, ss)
}

func onClosedStr(Q chan<- string, s string, p Process) {
	<-p.Closed()
	Q <- s
}
