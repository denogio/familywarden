#!/usr/bin/env python3
"""Check icon-only navigation in a UIAutomator dump from fictional Compose previews."""
import argparse
import math
import re
import xml.etree.ElementTree as ET


def check(root, labels, selected, density):
    parents = {child: parent for parent in root.iter() for child in parent}

    def bounds(node):
        result = tuple(map(int, re.findall(r'-?\d+', node.get('bounds', ''))))
        assert len(result) == 4, node.attrib
        return result

    nav = []
    for label in labels:
        matches = [n for n in root.iter('node') if n.get('content-desc') == label]
        assert matches, f'No accessible navigation name: {label}'
        # The item container is clickable; Android reports the already selected item
        # as selected but not clickable, because tapping it does nothing.
        target = matches[0]
        while target.get('clickable') != 'true' and target.get('selected') != 'true' and target in parents:
            target = parents[target]
        assert target.get('clickable') == 'true' or target.get('selected') == 'true', \
            f'Navigation is not an item: {label}'
        x1, y1, x2, y2 = bounds(target)
        minimum = math.ceil(48 * density / 160)
        assert x2 - x1 >= minimum and y2 - y1 >= minimum, f'Touch target below 48 dp: {label}'
        assert (target.get('selected') == 'true') == (label == selected), f'Wrong selected state: {label}'
        nav.append(target)

    top = min(bounds(n)[1] for n in nav)
    height = max(bounds(n)[3] for n in root.iter('node'))
    assert top > height / 2, 'Navigation is not at the bottom'
    assert len({bounds(n) for n in nav}) == len(labels), 'Navigation items overlap'
    for n in root.iter('node'):
        if bounds(n)[1] >= top:
            assert n.get('text', '') == '', f'Visible navigation label: {n.get("text")}'
    assert any(n.get('text') == selected and bounds(n)[3] < top for n in root.iter('node')), \
        f'Visible page heading missing: {selected}'


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('xml')
    parser.add_argument('--labels', nargs='+', required=True)
    parser.add_argument('--selected', required=True)
    parser.add_argument('--density', type=int, required=True)
    args = parser.parse_args()
    check(ET.parse(args.xml).getroot(), args.labels, args.selected, args.density)
    print(f'PASS icon-only navigation: {args.selected} ({len(args.labels)} accessible targets)')
