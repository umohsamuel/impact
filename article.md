# I Built an API That Adds Impact Effects to Fight Videos Using AI

Stolen idea btw. Shoutout to [Pr3c10us](https://github.com/Pr3c10us) for the original concept. This was supposed to be a collab but we decided to build our separate implementations instead.

The goal: take a fight video (anime, boxing, UFC, whatever) and automatically add those dramatic black and white flash effects at the exact moments where someone gets hit. You know, the ones you see in anime edits and fight compilations on TikTok. The kind where the frame goes high contrast and inverted for a split second right when a punch connects.

The idea is simple: upload a video, let AI figure out where the hits land, apply the effects, get the video back.

Here is how I built it.

**Before:**

<video src="https://res.cloudinary.com/db6nohcui/video/upload/v1777318528/before-impact-demo_fpehmn.mp4" controls muted playsinline width="100%"></video>

**After:**

<video src="https://res.cloudinary.com/db6nohcui/video/upload/v1777318152/after-impact-demo_xkxv5d.mp4" controls muted playsinline width="100%"></video>

## The Problem

Doing this manually in a video editor is tedious. You have to scrub through the video frame by frame, find every impact moment, add your effect, adjust the timing, repeat. For a 30 second clip with 5 impacts that might take 15 minutes. For longer videos it gets worse.

I wanted a single API call that handles the whole thing.

## The Stack

- **Go** with Gin for the HTTP server
- **FFmpeg** for all the video processing (frame extraction, labeling, grid creation, effect application)
- **Google Gemini** (via the genai SDK) for analyzing frames and finding impact moments
- **PostgreSQL** with sqlc for the database layer

The project follows a hexagonal architecture. Ports, adapters, domain, services. Keeps things clean and testable.

## Step 1: Extract and Label Frames

The first thing the pipeline does after receiving a video upload is extract frames at 10 frames per second. Each frame gets a visible number burned into the top left corner. This number is how the AI will reference specific frames later.

```go
func ExtractAndLabelFrames(inputPath, outputDir string, sampleRate float64) (*ExtractedFrames, error) {
    if err := os.MkdirAll(outputDir, 0755); err != nil {
        return nil, fmt.Errorf("failed to create output dir: %w", err)
    }

    filterStr := fmt.Sprintf(
        "fps=%g,scale=640:-2,drawtext=text='%%{eif\\:n+1\\:d}':x=10:y=10:fontsize=28:fontcolor=white:borderw=2:bordercolor=black",
        sampleRate,
    )
    outputPattern := filepath.Join(outputDir, "frame_%05d.jpg")

    args := []string{
        "-y",
        "-i", inputPath,
        "-vf", filterStr,
        "-q:v", "3",
        "-f", "image2",
        outputPattern,
    }

    cmd := exec.Command("ffmpeg", args...)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return nil, fmt.Errorf("ffmpeg extract+label error: %w\noutput: %s", err, string(output))
    }

    // ... glob and sort the frame paths, get metadata
}
```

The `drawtext` filter here uses `%{eif\:n+1\:d}` to burn the frame number as an integer. I initially used `%{expr\:n+1}` but that gave me numbers like `1.000000` instead of just `1`. The `eif` variant formats it as a decimal integer.

For a 30 second video at 10fps, this gives you 300 labeled frames.

![Labeled frame example](https://res.cloudinary.com/db6nohcui/image/upload/v1777318431/portfiolio-blog/o6kdfypucaxuxet6v7fn.jpg)

## Step 2: Arrange Frames Into Grids

Sending 300 individual images to Gemini would be expensive and slow. Instead, I tile the frames into 5x4 grids. That is 20 frames per grid image. A 30 second video now becomes about 15 grid images instead of 300 individual frames.

```go
const (
    gridCols      = 5
    gridRows      = 4
    framesPerGrid = gridCols * gridRows
)
```

Each grid is built using FFmpeg's concat demuxer and tile filter:

```go
func createSingleGrid(framePaths []string, outputPath string) error {
    // Write a concat list so ffmpeg reads the frames as a sequence
    listPath := outputPath + ".txt"
    var listContent strings.Builder
    for _, p := range framePaths {
        abs, _ := filepath.Abs(p)
        listContent.WriteString(fmt.Sprintf("file '%s'\n", filepath.ToSlash(abs)))
        listContent.WriteString("duration 0.04\n")
    }
    os.WriteFile(listPath, []byte(listContent.String()), 0644)

    tileLayout := fmt.Sprintf("%dx%d", gridCols, gridRows)

    args := []string{
        "-y",
        "-f", "concat",
        "-safe", "0",
        "-i", listPath,
        "-vf", fmt.Sprintf("scale=640:-2,tile=%s", tileLayout),
        "-frames:v", "1",
        "-q:v", "3",
        outputPath,
    }

    cmd := exec.Command("ffmpeg", args...)
    // ...
}
```

If the last batch has fewer than 20 frames, the tile filter fills the empty cells with black. No special handling needed.

![5x4 grid of labeled frames](https://res.cloudinary.com/db6nohcui/image/upload/v1777318375/portfiolio-blog/hdqvgsz2o3uorzaddclk.jpg)

## Step 3: Let Gemini Find the Impacts

The grid images get uploaded to Gemini via the File API. Then I send a prompt that tells the model exactly what to look for and what to ignore.

The system prompt is pretty specific:

```markdown
A key action moment is when two things make significant physical contact:

- A character's motion connecting with another character or object
- A fast-moving object reaching its target
- Two characters or objects colliding with visible force
- A character making contact with a surface with force
```

And equally specific about what to skip:

```markdown
- Movement without any contact (running, jumping, flying)
- Preparation or wind-up before contact happens
- Characters standing, posing, talking, or reacting
- The aftermath of contact (character already moving away)
- Black screens or dark frames between scenes
- Scene transitions, fades, cuts between angles
```

The model returns a simple JSON response:

```json
{ "impacts": [12, 45, 78, 132] }
```

Just frame numbers. Nothing else. The prompt explicitly says to pick only the 2 to 5 best moments. Quality over quantity.

The grids are uploaded concurrently with a buffered channel to avoid hammering the API:

```go
const maxConcurrent = 5
sem := make(chan struct{}, maxConcurrent)
ch := make(chan result, len(files))

for i, f := range files {
    sem <- struct{}{}
    go func(i int, f llm.File) {
        defer func() { <-sem }()
        uf, err := g.uploadFile(ctx, f)
        ch <- result{index: i, file: uf, err: err}
    }(i, f)
}
```

## Step 4: Apply the Effects

This is the interesting part. The original approach was to re-extract frames from the video at 10fps, apply the effect to the impact frames individually, then stitch everything back together. That worked but the output was choppy because you are reconstructing a video at 10fps regardless of what the original framerate was.

The fix: skip the frame extraction entirely. Convert the impact frame numbers to timestamps and apply the effect filters directly to the original video. FFmpeg has an `enable` parameter on its filters that lets you turn them on and off based on time.

```go
func BuildImpactVideo(inputPath, outputDir string, impactFrames []int, sampleRate float64, meta *VideoMetadata) (string, error) {
    // Convert frame numbers to time windows
    type timeWindow struct {
        start float64
        end   float64
    }

    var windows []timeWindow
    for _, f := range impactFrames {
        startFrame := f
        if startFrame < 1 {
            startFrame = 1
        }
        endFrame := f + 1

        startTime := float64(startFrame-1) / sampleRate
        endTime := float64(endFrame) / sampleRate
        windows = append(windows, timeWindow{startTime, endTime})
    }

    // Sort and merge overlapping windows
    sort.Slice(windows, func(i, j int) bool { return windows[i].start < windows[j].start })
    var merged []timeWindow
    for _, w := range windows {
        if len(merged) > 0 && w.start <= merged[len(merged)-1].end {
            if w.end > merged[len(merged)-1].end {
                merged[len(merged)-1].end = w.end
            }
        } else {
            merged = append(merged, w)
        }
    }

    // Build enable expression
    var parts []string
    for _, w := range merged {
        parts = append(parts, fmt.Sprintf("between(t,%.3f,%.3f)", w.start, w.end))
    }
    enableExpr := strings.Join(parts, "+")

    filter := fmt.Sprintf(
        "hue=s=0:enable='%s',eq=contrast=3.5:brightness=0.9:enable='%s',curves=all='0/0 0.25/0.1 0.5/0.6 0.75/0.95 1/1':enable='%s'",
        enableExpr, enableExpr, enableExpr,
    )

    // ... run ffmpeg with the filter
}
```

The `enable='between(t,4.000,4.200)'` part tells FFmpeg to only apply that filter between 4.0 and 4.2 seconds. The rest of the video passes through untouched. This means the output keeps the original video's native framerate. If your input is 60fps, the output is 60fps. No choppiness.

The effect itself is a chain of three filters:

1. `hue=s=0` removes all color (full desaturation)
2. `eq=contrast=3.5:brightness=0.9` cranks the contrast way up
3. `curves` applies a custom tone curve for that punchy look

## The Full Pipeline

Putting it all together, the handler orchestrates everything in one request:

```go
func (h *Handler) ExtractFrames(c *gin.Context) {
    // 1. Receive video upload (max 1GB)
    file, err := c.FormFile("video")

    // 2. Extract labeled frames at 10fps
    extracted, err := videoService.ExtractAndLabelFrames(inputPath, framesDir, sampleRate)

    // 3. Create 5x4 grids
    gridPaths, err := videoService.CreateGridImages(extracted.Paths, gridsDir)

    // 4. Upload grids to Gemini
    uploadedFiles, err := h.llm.UploadFiles(llmFiles)

    // 5. Send prompt + grids to Gemini
    llmResponse, err := h.llm.GenerateText(fullPrompt, false, uploadedFiles)

    // 6. Parse the frame numbers from the response
    var impactResponse domainVideo.ImpactResponse
    json.Unmarshal([]byte(jsonStr), &impactResponse)

    // 7. Build the output video with effects applied
    outputPath, err := videoService.BuildImpactVideo(inputPath, tempDir, impactResponse.Impacts, sampleRate, meta)

    // 8. Return download URL
    response.NewSuccessResponse("Impact video generated successfully", gin.H{
        "download_url": downloadURL,
        "impacts":      impactResponse.Impacts,
        "llm_cost":     llmResponse.Dollars,
    }, nil).Send(c)
}
```

Upload a video, get back a download URL. The response also includes which frame numbers were detected and how much the Gemini API call cost.

## Example Output

**Before (original):**

<video src="https://res.cloudinary.com/db6nohcui/video/upload/v1777318528/before-impact-demo_fpehmn.mp4" controls muted playsinline width="100%"></video>

**After (with impact effects):**

<video src="https://res.cloudinary.com/db6nohcui/video/upload/v1777318152/after-impact-demo_xkxv5d.mp4" controls muted playsinline width="100%"></video>

## Things I Ran Into

**FFmpeg escaping on Windows.** The filter strings have nested single quotes (the `enable='...'` and `curves=all='...'` both use them). On Windows, the command line argument parsing was mangling them. I ended up writing the filter to a file and using `-filter_script:v` to load it, which bypasses all the escaping issues.

**Label formatting.** FFmpeg's `drawtext` expression `%{expr\:n+1}` gives you `1.000000`. You need `%{eif\:n+1\:d}` for integer formatting.

**Grid rendering.** I initially tried using FFmpeg's `xstack` filter to arrange frames into grids. It was unreliable. Switching to the `concat` demuxer plus the `tile` filter was way more straightforward.

**Gemini content filtering.** Google's safety filters can block fight/combat content at the infrastructure level, even with all safety settings set to off. This is a policy level thing, not something you can override via the API. Worth knowing if you are working with action content.

**Thinking config compatibility.** `gemini-2.5-pro` does not support `ThinkingConfig`. Only the preview variants do. If you set `ThinkingLevel: ThinkingLevelHigh` on the non-preview model, you get a 400 error.

## Try It Out

The full source code is on GitHub: [github.com/umohsamuel/impact](https://github.com/umohsamuel/impact)

You need Go, FFmpeg, and a Google Gemini API key to run it locally.

```bash
git clone https://github.com/umohsamuel/impact.git
cd impact
go mod download
# set up your .env
go build -o ./tmp/main.exe ./cmd
./tmp/main.exe
```

Then hit the endpoint:

```bash
curl -X POST http://localhost:5000/api/v1/generate-impact-frames \
  -F "video=@your-video.mp4"
```
