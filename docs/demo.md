# Public demo

The demo runs at [bookmarkbox.onrender.com](https://bookmarkbox.onrender.com) on Render's free tier, from `render.yaml` at the root of the repository. [Design](design.md#public-demo) explains how demo mode works and why; this page covers deploying and checking it.

## Settings

`render.yaml` sets up one free Docker web service, built from the repository's `Dockerfile`, in Frankfurt. The region can't be changed once the service exists.

| Setting | Value | Why |
|---|---|---|
| `DEMO_MODE` | `true` | Resets the data at start-up and every 24 hours, and shows the demo's notices |
| `MAX_USERS` | `25` | Includes the Demo User |
| `MAX_BOOKMARKS_PER_USER` | `300` | The seed has 232 |
| `MAX_TAGS_PER_USER` | `300` | |
| `CLIENT_IP_HEADER` | `True-Client-IP` | Render's proxy puts the visitor's address here. See [Check the client IP](#check-the-client-ip) |
| `DATABASE_PATH` | `/data/bookmarkbox.db` | The image's default, set here so it's visible |
| `GOMEMLIMIT` | `400MiB` | Keeps Go's memory below the free tier's 512 MB |

- Render sets `PORT` itself, and the app reads it.
- The health check is `/ping`.
- Deploys happen when `main` changes and its CI checks pass (`autoDeployTrigger: checksPass`).
- There's no disk, because free services can't have one and the demo resets anyway.
- There are no secrets. The Demo login is shown on the home page.
- `COOKIE_SECURE` stays at its default, so session cookies are HTTPS only.

With `DEMO_MODE` on, the app won't start if a limit is `0` or `CLIENT_IP_HEADER` is unset.

## Deploy

1. In the Render dashboard, choose **New > Blueprint**, and connect the GitHub repository.
2. Render reads `render.yaml` from `main` and lists one service, `bookmarkbox`. Apply it.
3. The first build takes a few minutes. The address is `https://bookmarkbox.onrender.com`, or that name with a suffix if it was taken.

Later changes to `render.yaml` on `main` are applied by the Blueprint's next sync.

## Check it's live

```bash
DEMO=https://bookmarkbox.onrender.com
curl -si $DEMO/ping | head -1                                # HTTP/2 200
curl -sI $DEMO/ | grep -i strict-transport-security          # max-age=31536000
curl -s $DEMO/ | grep -c 'Log in as Demo'                    # 1
```

Then, in a browser:

- The banner says this is a public demo.
- **Log in as Demo** shows about 230 Bookmarks, the newest dated today.
- The Account page says the Demo password can't be changed.
- Sign up as a visitor, add a Bookmark, and log out.

Render's **Logs** tab should show `demo data reset` at start-up, with the time of the next reset.

## Check the client IP

Rate limits count each visitor by IP address, so the app has to read the right one. Render runs behind Cloudflare, which sets `True-Client-IP`, but Render doesn't document that header, so check it after each change of host or settings.

1. Find your address: `curl -s https://ifconfig.me`.
2. Open the demo, and find your request in Render's logs. Its `ip` should be your address.
3. Try to fake it:
   ```bash
   curl -s -o /dev/null -H 'True-Client-IP: 203.0.113.9' -H 'X-Forwarded-For: 203.0.113.9' $DEMO/ping
   ```
   The log should still show your address, not `203.0.113.9`.
4. A warning that the client IP header is missing or invalid means Render no longer sets it. Then try `CLIENT_IP_HEADER=X-Forwarded-For`, whose last entry the app uses, and repeat the checks.

Rate limits refill one attempt every 6 seconds, and each login takes about 1.5 seconds on the free tier, so attempts sent one at a time barely go over. Send them a few at once instead, for example `seq 1 14 | xargs -P 3 -I{} curl …`. The 14th should get a 429.

Last checked on 2026-11-12: `True-Client-IP` held the visitor's address, and fake `True-Client-IP` and `X-Forwarded-For` headers were ignored.

The demo's address is IPv4 only, so a visitor's log entry shows their IPv4 address even if their machine prefers IPv6.

## Reset by hand

Any restart resets the data. In the dashboard, use **Manual Deploy > Deploy latest commit**, or **Restart service**.

## Cold starts

After 15 minutes without traffic the service spins down. The next visit waits about a minute while it starts, then gets fresh data. Render may also restart a free service at any time.

Free services get 750 instance hours a month for each workspace, enough for one service running all month, but not two. A pinger could keep the service awake to avoid cold starts, though the repository doesn't run one. Steady traffic also stops the spin-downs that reset the data, which is why demo mode resets every 24 hours as well.
