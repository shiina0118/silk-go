package silk

import (
	"math"
)

func combine_and_check(
	pulses_comb, pulses_in *slice[int32],
	max_pulses, len int32) int32 {
	return combineAndCheck(pulses_comb.From(), pulses_in.From(), max_pulses, len)
}

func combineAndCheck(pulses_comb, pulses_in []int32, max_pulses, len int32) int32 {
	var k, sum int32

	for k = 0; k < len; k++ {
		sum = pulses_in[2*k] + pulses_in[2*k+1]
		if sum > max_pulses {
			return 1
		}
		pulses_comb[k] = sum
	}

	return 0
}

func encode_pulses(psRC *range_coder_state, sigtype, QuantOffsetType int32, q *slice[int8], frame_length int32) {
	var (
		i, k, j, iter, bit, nLS, scale_down, RateLevelIndex int32
		abs_q, minSumBits_Q6, sumBits_Q6                    int32
		abs_pulses                                          [MAX_FRAME_LENGTH]int32
		sum_pulses                                          [MAX_NB_SHELL_BLOCKS]int32
		nRshifts                                            [MAX_NB_SHELL_BLOCKS]int32
		pulses_comb                                         [8]int32
		abs_pulses_ptr                                      []int32
		pulses_ptr                                          []int8
		cdf_ptr                                             []uint16
		nBits_ptr                                           []int16
	)
	samples := q.From()

	iter = frame_length / SHELL_CODEC_FRAME_LENGTH

	for i = 0; i < frame_length; i += 4 {
		abs_pulses[i] = abs(int32(samples[i]))
		abs_pulses[i+1] = abs(int32(samples[i+1]))
		abs_pulses[i+2] = abs(int32(samples[i+2]))
		abs_pulses[i+3] = abs(int32(samples[i+3]))
	}

	for i = 0; i < iter; i++ {
		abs_pulses_ptr = abs_pulses[i*SHELL_CODEC_FRAME_LENGTH : (i+1)*SHELL_CODEC_FRAME_LENGTH]

		for {
			scale_down = combineAndCheck(pulses_comb[:], abs_pulses_ptr, max_pulses_table[0], 8)

			scale_down += combineAndCheck(pulses_comb[:], pulses_comb[:], max_pulses_table[1], 4)

			scale_down += combineAndCheck(pulses_comb[:], pulses_comb[:], max_pulses_table[2], 2)

			sum_pulses[i] = pulses_comb[0] + pulses_comb[1]
			if sum_pulses[i] > max_pulses_table[3] {
				scale_down++
			}

			if scale_down != 0 {
				nRshifts[i]++
				for k = 0; k < SHELL_CODEC_FRAME_LENGTH; k++ {
					abs_pulses_ptr[k] = RSHIFT(abs_pulses_ptr[k], 1)
				}
			} else {
				break
			}
		}
	}

	minSumBits_Q6 = math.MaxInt32
	for k = 0; k < N_RATE_LEVELS-1; k++ {
		nBits_ptr = pulses_per_block_BITS_Q6[k]
		sumBits_Q6 = int32(rate_levels_BITS_Q6[sigtype][k])
		for i = 0; i < iter; i++ {
			if nRshifts[i] > 0 {
				sumBits_Q6 += int32(nBits_ptr[MAX_PULSES+1])
			} else {
				sumBits_Q6 += int32(nBits_ptr[sum_pulses[i]])
			}
		}
		if sumBits_Q6 < minSumBits_Q6 {
			minSumBits_Q6 = sumBits_Q6
			RateLevelIndex = k
		}
	}

	range_encoder(psRC, RateLevelIndex, rate_levels_CDF[sigtype])

	cdf_ptr = pulses_per_block_CDF[RateLevelIndex]
	for i = 0; i < iter; i++ {
		if nRshifts[i] == 0 {
			range_encoder(psRC, sum_pulses[i], cdf_ptr)
		} else {
			range_encoder(psRC, MAX_PULSES+1, cdf_ptr)
			for k = 0; k < nRshifts[i]-1; k++ {
				range_encoder(psRC, MAX_PULSES+1, pulses_per_block_CDF[N_RATE_LEVELS-1])
			}
			range_encoder(psRC, sum_pulses[i], pulses_per_block_CDF[N_RATE_LEVELS-1])
		}
	}

	for i = 0; i < iter; i++ {
		if sum_pulses[i] > 0 {
			shellEncoder(psRC, abs_pulses[i*SHELL_CODEC_FRAME_LENGTH:])
		}
	}

	for i = 0; i < iter; i++ {
		if nRshifts[i] > 0 {
			pulses_ptr = samples[i*SHELL_CODEC_FRAME_LENGTH:]
			nLS = nRshifts[i] - 1
			for k = 0; k < SHELL_CODEC_FRAME_LENGTH; k++ {
				abs_q = abs(int32(pulses_ptr[k]))
				for j = nLS; j > 0; j-- {
					bit = RSHIFT(abs_q, j) & 1
					range_encoder(psRC, bit, lsb_CDF)
				}
				bit = abs_q & 1
				range_encoder(psRC, bit, lsb_CDF)
			}
		}
	}

	encode_signs(psRC, q, frame_length, sigtype, QuantOffsetType, RateLevelIndex)
}
