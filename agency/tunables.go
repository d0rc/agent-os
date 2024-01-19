package agency

import (
	os_client "github.com/d0rc/agent-os/os-client"
	"time"
)

const NumberOfVotesToCache = 3
const VoterMinResults = 1
const MinimumNumberOfVotes = 1
const MinimalVotingRatingForCommand = -1
const ToTPathLenToTriggerTerminalCallback = 18
const ResubmitSystemPromptAfter = 15 * time.Minute
const MaxIoRequestsThreads = 160
const WriteVotesLog = true

const MaxJobsPerAgent = 160
const JobsManagerInferenceTimeout = 600 * time.Second
const JobsManagerExecutionPool = os_client.REP_Default
