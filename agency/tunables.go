package agency

import (
	"github.com/d0rc/agent-os/stdlib/os-client"
	"time"
)

const NumberOfVotesToCache = 2
const VoterMinResults = 6
const MinimumNumberOfVotes = VoterMinResults
const MinimalVotingRatingForCommand = 3
const MaxIoRequestsThreads = 160
const WriteVotesLog = true

const MaxJobsPerAgent = 16
const JobsManagerInferenceTimeout = 600 * time.Second
const JobsManagerExecutionPool = os_client.REP_Default

const ShouldWriteMessageTrace = false
