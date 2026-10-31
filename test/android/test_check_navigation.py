#!/usr/bin/env python3
"""Small offline checks for the labelled UIAutomator navigation validator."""
import unittest
import xml.etree.ElementTree as ET

from check_navigation import check


class NavigationCheckTest(unittest.TestCase):
    def setUp(self):
        self.labels = ['Screen time', 'Controls', 'Requests']
        self.root = ET.Element('hierarchy')
        ET.SubElement(self.root, 'node', text='Screen time', bounds='[0,0][360,50]')
        for i, label in enumerate(self.labels):
            item = ET.SubElement(self.root, 'node', **{
                'text': label, 'clickable': 'true',
                'selected': str(i == 0).lower(),
                'bounds': f'[{i * 120},720][{(i + 1) * 120},800]',
            })
            if label == 'Requests':
                ET.SubElement(item, 'node', text='3', bounds='[300,720][320,740]')

    def validate(self):
        check(self.root, self.labels, 'Screen time', 160)

    def test_labelled_items_with_numeric_badge_pass(self):
        self.validate()

    def test_merged_badge_and_label_pass(self):
        self.root[3].set('text', '3, Requests')
        self.validate()

    def test_icon_only_item_is_rejected(self):
        self.root[2].set('text', '')
        self.root[2].set('content-desc', 'Controls')
        with self.assertRaisesRegex(AssertionError, 'Missing visible navigation label'):
            self.validate()

    def test_label_read_twice_is_rejected(self):
        self.root[2].set('content-desc', 'Controls')
        with self.assertRaisesRegex(AssertionError, 'read twice'):
            self.validate()

    def test_missing_accessible_name_is_rejected(self):
        self.root[2].set('text', '')
        with self.assertRaisesRegex(AssertionError, 'accessible navigation name'):
            self.validate()

    def test_unexpected_navigation_text_is_rejected(self):
        ET.SubElement(self.root[2], 'node', text='Beta', bounds='[130,720][150,740]')
        with self.assertRaisesRegex(AssertionError, 'Unexpected navigation text'):
            self.validate()

    def test_wrong_selection_is_rejected(self):
        self.root[2].set('selected', 'true')
        with self.assertRaisesRegex(AssertionError, 'Wrong selected state'):
            self.validate()

    def test_small_target_is_rejected(self):
        self.root[2].set('bounds', '[120,720][140,740]')
        with self.assertRaisesRegex(AssertionError, 'Touch target'):
            self.validate()

    def test_missing_heading_is_rejected(self):
        self.root[0].set('text', '')
        with self.assertRaisesRegex(AssertionError, 'heading missing'):
            self.validate()


if __name__ == '__main__':
    unittest.main()
