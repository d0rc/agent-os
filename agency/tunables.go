package agency

import "time"

const NumberOfVotesToCache = 5
const VoterMinResults = 7
const MinimalVotingRatingForCommand = 5.0
const ToTPathLenToTriggerTerminalCallback = 5
const ResubmitSystemPromptAfter = 15 * time.Minute
const MaxIoRequestsThreads = 128
