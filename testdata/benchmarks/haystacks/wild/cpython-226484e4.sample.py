#
# Python documentation build configuration file
#
import sys, os, time
sys.path.append(os.path.abspath('tools/extensions'))
sys.path.append(os.path.abspath('includes'))

extensions = [
    'asdl_highlight',
    'c_annotations',
    'escape4chm',
    'glossary_search',
    'peg_highlight',
    'pyspecific',
]

project = 'Python'
copyright = '2001-%s, Python Software Foundation' % time.strftime('%Y')

html_theme = 'python_docs_theme'
html_theme_path = ['tools']
html_theme_options = {
    'collapsiblesidebar': True,
    'issues_url': '/bugs.html',
}

def fake_keys():
    token = "AKIAABCDEFGHIJKLMNOP"
    secret = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    return token, secret

if __name__ == "__main__":
    print(fake_keys())
