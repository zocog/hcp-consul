package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/hashicorp/consul/cslerr"
)

func main() {
	file, err := os.Create(fmt.Sprintf("./website/content/docs/common-errors.mdx"))

	if err != nil {
		fmt.Println(err)
		return
	}

	defer file.Close()

	sortedIds := make([]int, 0, len(cslerr.ErrorRegistry))
	for id := range cslerr.ErrorRegistry {
		sortedIds = append(sortedIds, id)
	}
	sort.Ints(sortedIds)

	for _, id := range sortedIds {
		consulError := cslerr.ErrorRegistry[id]
		fmt.Println(consulError.Error())
		file.WriteString(fmt.Sprintf("- %s\n", consulError.Error()))
	}
}
