from setuptools import setup
from wheel.bdist_wheel import bdist_wheel


class WindowsCtypesWheel(bdist_wheel):
    def get_tag(self):
        return "py3", "none", "win_amd64"


setup(cmdclass={"bdist_wheel": WindowsCtypesWheel})
