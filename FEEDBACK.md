The more complete report is in REPORT.md
These are my own thoughts though.

Try it at:
https://silson.dev/docs

HOW TO READ THIS DOCUMENT:\
⭐ These are points that show particular senior experience or judgement.

# 🧠 AI workflow
My appraoch as to feed the problem to Claude and then start discussing strategy and how much I wanted to implement. As a senior candidate I aimed for and implemented Task 1,2,3 as well as some of 4. I chose to leave out Oauth to save some time and remove the need for an external integration.

I batched the overall project into multiple phases and then reviewed the results of each phase. I chose what features to do in what order and asked for each feature to be it's own self-contained commit so I could review it.

# 🖥️ Infrastructure
My first step was to nail down hosting which can be a bottleneck in any project and I did not want to leave that until last. 
So I spun up the droplet, connected my domain and ensured I had a certificate from LetsEncrypt 🔒 

⭐ I scanned it on Qualys and got an A rating\
 https://www.ssllabs.com/ssltest/analyze.html?d=silson.dev

What kept me from A+ was that I did not have post-quantum cryptography or DNS CAA (the latter would not have helped with the eMudhra situation).

# ⚙️ Functionality
The basic CRUD  was fairly straghtforward.Nothing super interesting to report.

🏢 **Multi-tenancy**\
I made the decision up front to implement multi tenancy from the start, because it would be difficult to retrofit later.
A naive approach would be to just trust the service code to always implement a WHERE tenant=... clause everwhere it was needed.

Since you specifically asked for the feature I assumed this would not be enough, so I implemented Row Level Security. This means that the partitioning lives in the database rather than app code. This was not entirely free, while most requests would be relatively unaffected, the search was slightly slowed. This was mitigated by pre-processing on write though.

⭐ The main question when implementing a feature like this is "what are we defending against" vs "what are the consequences of failure".
- a bug in the single service that uses this db? Arguably overkill. 
- a hacker? Probably...
- a database that is shared by multiple services where clients have sensitive data. Definitely WORTH IT.

## 🎭 Sentiment Analysis
I implemented a simple word list based sentiment analyser. It is behind an interface though, so at a later time it could be hooked up to something better. This is assuming a fast enough synchronous one could be found. If the sentiment analysis is too slow then new reviews could be added to a queue and score offline.

# 🚀 Performance (and monitoring)
Task 3 said the service was running on an old server....\
Honestly though, even the cheapest droplet tier performed quite respectably being able to serve around 300 rps.
It turned out that the limitation was actually the kernels connection tracking and NGINX default connection handling. Once that was fixed we got to see the caching help reduce the median dramatically.

Results can be seen here:
https://bigwoodland1718.grafana.net/dashboard/snapshot/dsvbgwCK2Ni7vXSwYmCrsYFlO0YQ5dOE


## Rate limiting
Per-customer rate limit is done using an in memory cache which saves db work. 

Brute force protection throttles the user rather than locking them out.
