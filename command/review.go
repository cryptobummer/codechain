package command

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/frankbraun/codechain/hashchain"
	"github.com/frankbraun/codechain/tree"
	"github.com/frankbraun/codechain/util/git"
	"github.com/frankbraun/codechain/util/interrupt"
	"github.com/frankbraun/codechain/util/terminal"
)

func review(c *hashchain.HashChain, secKeyFile string, verbose bool) error {
	// load secret key
	secKey, _, _, err := seckeyLoad(c, secKeyFile)
	if err != nil {
		return err
	}

	// get last tree hashes
	signedTreeHash, idx := c.LastSignedTreeHash()
	if signedTreeHash == c.LastTreeHash() {
		fmt.Printf("%s: already signed\n", signedTreeHash)
		return nil
	}
	treeHashes := c.TreeHashes()
	treeComments := c.TreeComments()
	if len(treeHashes) != len(treeComments) {
		return fmt.Errorf("invariant failed: len(treeHashes) == len(treeComments)")
	}

	// TODO: also show commits which have been signed, but not by this signer
	// TODO: deal with explicit treehash
	// TODO: show changes in signers/sigctl!

	for i := idx; i < len(treeHashes)-1; i++ {
		// bring .codechain/tree/a in sync
		err = tree.Sync(treeDirA, treeHashes[i], patchDir, treeHashes, verbose, excludePaths)
		if err != nil {
			return err
		}

		// bring .codechain/tree/b in sync
		err = tree.Sync(treeDirB, treeHashes[i+1], patchDir, treeHashes, verbose, excludePaths)
		if err != nil {
			return err
		}

		// show patches info
		pub, comment := c.SignerInfo(treeHashes[i+1])
		fmt.Printf("patch %d/%d\n", i-idx+1, len(treeHashes)-idx-1)
		if treeComments[i] != "" {
			fmt.Println(treeComments[i])
		}
		fmt.Printf("developer: %s\n", pub)
		if comment != "" {
			fmt.Println(comment)
		}
		for {
			fmt.Print("review patch? [y/n]: ")
			answer, err := terminal.ReadLine(os.Stdin)
			if err != nil {
				return err
			}
			a := string(bytes.ToLower(answer))
			if strings.HasPrefix(a, "y") {
				break
			} else if strings.HasPrefix(a, "n") {
				return errors.New("aborted")
			} else {
				fmt.Println("answer not recognized")
			}
		}

		// display diff *pager
		if err := git.DiffPager(treeDirA, treeDirB); err != nil {
			return err
		}

		// confirm patch
		for {
			fmt.Print("sign patch? [y/n]: ")
			answer, err := terminal.ReadLine(os.Stdin)
			if err != nil {
				return err
			}
			a := string(bytes.ToLower(answer))
			if strings.HasPrefix(a, "y") {
				break
			} else if strings.HasPrefix(a, "n") {
				return errors.New("aborted")
			} else {
				fmt.Println("answer not recognized")
			}
		}
	}

	// sign patches and add to hash chain
	entry, err := c.Signature(c.LastEntryHash(), *secKey)
	if err != nil {
		return err
	}
	fmt.Println(entry)
	return nil
}

// Review implements the 'review' command.
func Review(argv0 string, args ...string) error {
	fs := flag.NewFlagSet(argv0, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [-s seckey.bin] [treehash]\n", argv0)
		fmt.Fprintf(os.Stderr, "Review code changes (all or up to treehash) and changes of signers and sigctl.\n")
		fs.PrintDefaults()
	}
	seckey := fs.String("s", "", "Secret key file")
	verbose := fs.Bool("v", false, "Be verbose")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := seckeyCheck(*seckey); err != nil {
		return err
	}
	if fs.NArg() != 0 && fs.NArg() != 1 {
		fs.Usage()
		return flag.ErrHelp
	}
	if err := os.MkdirAll(treeDirA, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(treeDirB, 0755); err != nil {
		return err
	}
	/* TODO
	var treehash string
	if fs.NArg() == 1 {
		treehash = fs.Arg(0)
		// TODO
		return errors.New("not implemented")
	}
	*/
	c, err := hashchain.Read(hashchainFile)
	if err != nil {
		return err
	}
	defer c.Close()
	// add interrupt handler
	interrupt.AddInterruptHandler(func() {
		c.Close()
	})
	// run review
	go func() {
		if err := review(c, *seckey, *verbose); err != nil {
			interrupt.ShutdownChannel <- err
			return
		}
		interrupt.ShutdownChannel <- nil
	}()
	return <-interrupt.ShutdownChannel
}
