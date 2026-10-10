package silk

func inner_prod_aligned(inVec1, inVec2 *slice[int16], len int32) int32 {
	return innerProdAligned(inVec1.From(), inVec2.From(), len)
}

func innerProdAligned(input1, input2 []int16, length int32) int32 {
	if length <= 0 {
		return 0
	}
	input1 = input1[:length]
	input2 = input2[:length]
	var sum int32
	for i, sample := range input1 {
		sum = SMLABB(sum, int32(sample), int32(input2[i]))
	}
	return sum
}

func inner_prod16_aligned_64(inVec1, inVec2 *slice[int16], len int32) int64 {
	return innerProdAligned64(inVec1.From(), inVec2.From(), len)
}

func innerProdAligned64(input1, input2 []int16, length int32) int64 {
	if length <= 0 {
		return 0
	}
	input1 = input1[:length]
	input2 = input2[:length]
	var sum int64
	for i, sample := range input1 {
		sum = SMLALBB(sum, sample, input2[i])
	}
	return sum
}
