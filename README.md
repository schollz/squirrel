# squirrel

<a href="https://github.com/schollz/squirrel/releases/latest"><img src="https://img.shields.io/badge/version-v1.0.0-brightgreen.svg?style=flat-square" alt="Version"></a>
<a href="https://travis-ci.org/schollz/squirrel"><img
src="https://img.shields.io/travis/schollz/squirrel.svg?style=flat-square" alt="Build
Status"></a> 


Downloading the web can be cumbersome if you end up with thousands or millions of files. This tool allows you to download websites directly into a file-based database in SQLite, since [SQlite performs faster than a filesystem](https://www.sqlite.org/fasterthanfs.html) for reading and writing.


## Install

Download latest release:

```
$ curl https://raw.githubusercontent.com/schollz/squirrel/master/src/install/default.txt | bash
```

or build from source

```
$ go get -v github.com/schollz/squirrel
```


## Usage 

### Basic usage

It should be compatible with Firefox's "Copy as cURL", just replace `curl` with `squirrel`. By default it will save the data in a database, `urls.db`.

```
$ squirrel "https://www.sqlite.org/fasterthanfs.html" -H "User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:66.0) Gecko/20100101 Firefox/66.0" -H "Accept: text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8" -H "Accept-Language: en-US,en;q=0.5" --compressed -H "Referer: https://www.google.com/" -H "Connection: keep-alive" -H "Upgrade-Insecure-Requests: 1" -H "If-Modified-Since: Thu, 02 May 2019 16:25:06 +0000" -H "If-None-Match: ""m5ccb19e2s6076""" -H "Cache-Control: max-age=0"
```


## Contributing

Pull requests are welcome. Feel free to...

- Revise documentation
- Add new features
- Fix bugs
- Suggest improvements

## Thanks

Thanks Dr. H for the idea.

## License

MIT
